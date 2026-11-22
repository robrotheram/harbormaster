package setup

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type change struct {
	Target   string `json:"target"`
	Staged   string `json:"staged,omitempty"`
	Private  bool   `json:"private,omitempty"`
	Original bool   `json:"original"`
}

func (s Service) Apply(ctx context.Context, p *Plan) error {
	project, err := os.OpenRoot(s.project())
	if err != nil {
		return ErrStorage
	}
	defer project.Close()
	actual, err := s.fingerprint(ctx, project, p.inputs())
	if err != nil || actual != p.snapshot {
		return ErrChanged
	}
	state, err := os.OpenRoot(Dir(s.Home))
	if err != nil {
		return ErrStorage
	}
	defer state.Close()
	stage, err := os.OpenRoot(p.stage)
	if err != nil {
		return ErrStorage
	}
	defer stage.Close()

	var changes []change
	for _, name := range p.files {
		staged := name
		if _, err := cleanStat(stage, name); errors.Is(err, fs.ErrNotExist) {
			staged = ""
		} else if err != nil {
			return ErrStorage
		}
		if dir := path.Dir(name); dir != "." && staged != "" {
			if _, err := cleanStat(project, dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return ErrChanged
			}
			if project.MkdirAll(dir, 0755) != nil {
				return ErrStorage
			}
		}
		changes = append(changes, change{Target: name, Staged: staged})
	}
	if strings.HasPrefix(p.operation, "tool") {
		generation := "tools-" + strings.TrimPrefix(filepath.Base(p.stage), "plan-")
		if stage.Symlink(generation, "new-current") != nil {
			return ErrStorage
		}
		changes = append(changes, change{Target: generation, Staged: "active", Private: true}, change{Target: "current", Staged: "new-current", Private: true})
	}
	for i := range changes {
		target := project
		if changes[i].Private {
			target = state
		}
		if _, err := target.Lstat(changes[i].Target); err == nil {
			changes[i].Original = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return ErrStorage
		}
	}
	journal, err := json.Marshal(changes)
	if err != nil || writeSynced(stage, "journal.json", journal) != nil {
		return ErrStorage
	}
	if err := publish(stage, project, state, changes); err != nil {
		if rollback(stage, project, state, changes) != nil {
			return ErrStorage
		}
		_ = stage.Remove("journal.json")
		return ErrStorage
	}
	if err := writeSynced(stage, "committed", []byte("ok")); err != nil {
		if rollback(stage, project, state, changes) == nil {
			_ = stage.Remove("journal.json")
		}
		return ErrStorage
	}
	s.pruneToolPaths(state)
	return nil
}

func writeSynced(root *os.Root, name string, data []byte) error {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		_ = root.Remove(name)
	}
	return err
}

func publish(stage, project, state *os.Root, changes []change) error {
	for i, c := range changes {
		target := project
		if c.Private {
			target = state
		}
		if c.Original {
			if err := rename(target, c.Target, stage, backupName(i)); err != nil {
				return err
			}
		}
		if c.Staged != "" {
			if err := rename(stage, c.Staged, target, c.Target); err != nil {
				return err
			}
		}
	}
	return nil
}

func rollback(stage, project, state *os.Root, changes []change) error {
	var failure error
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		target := project
		if c.Private {
			target = state
		}
		if c.Staged != "" {
			if _, err := stage.Lstat(c.Staged); errors.Is(err, fs.ErrNotExist) {
				failure = errors.Join(failure, rename(target, c.Target, stage, c.Staged))
			}
		}
		if _, err := stage.Lstat(backupName(i)); err == nil {
			failure = errors.Join(failure, rename(stage, backupName(i), target, c.Target))
		}
	}
	return failure
}

func backupName(i int) string { return "backup-" + strconv.Itoa(i) }

func rename(from *os.Root, source string, to *os.Root, dest string) error {
	src, err := from.Open(path.Dir(source))
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := to.Open(path.Dir(dest))
	if err != nil {
		return err
	}
	defer dst.Close()
	return unix.Renameat2(int(src.Fd()), path.Base(source), int(dst.Fd()), path.Base(dest), unix.RENAME_NOREPLACE)
}

// Recover restores a prior checkout when a worker stopped partway through
// publication. It runs before the worker admits new processes.
func (s Service) Recover() error {
	state, err := os.OpenRoot(Dir(s.Home))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer state.Close()
	entries, err := fs.ReadDir(state.FS(), ".")
	if err != nil {
		return err
	}
	project, err := os.OpenRoot(s.project())
	if err != nil {
		return err
	}
	defer project.Close()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "plan-") {
			stage, err := state.OpenRoot(name)
			if err != nil {
				return err
			}
			err = recoverPlan(stage, project, state)
			stage.Close()
			if err != nil {
				return err
			}
		} else if !strings.HasPrefix(name, "discover-") && !strings.HasPrefix(name, "cli-tmp-") {
			continue
		}
		if err := state.RemoveAll(name); err != nil {
			return err
		}
	}
	s.pruneToolPaths(state)
	return nil
}

func recoverPlan(stage, project, state *os.Root) error {
	if _, err := stage.Stat("committed"); err == nil {
		return nil
	}
	raw, err := readFile(stage, "journal.json", metaLimit)
	if err != nil || raw == nil {
		return err
	}
	var changes []change
	if err := json.Unmarshal(raw, &changes); err != nil {
		return err
	}
	for _, c := range changes {
		if !validChange(c) {
			return ErrStorage
		}
	}
	return rollback(stage, project, state, changes)
}

func validChange(c change) bool {
	if c.Private {
		if c.Target == "current" {
			return c.Staged == "new-current"
		}
		return strings.HasPrefix(c.Target, "tools-") && path.Base(c.Target) == c.Target && c.Staged == "active"
	}
	if c.Staged != "" && c.Staged != c.Target {
		return false
	}
	switch c.Target {
	case "mise.toml", ".mise.toml", "mise.lock", skillLock:
		return true
	}
	return path.Dir(c.Target) == skillDir && skillName.MatchString(path.Base(c.Target))
}

func (s Service) pruneToolPaths(state *os.Root) {
	active, err := state.Readlink("current")
	if err != nil {
		return
	}
	entries, _ := fs.ReadDir(state.FS(), ".")
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "tools-") && entry.Name() != active {
			_ = state.RemoveAll(entry.Name())
		}
	}
}
