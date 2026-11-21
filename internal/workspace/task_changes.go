package workspace

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"agent-workbench/internal/shared"
)

const (
	gitSnapshotTimeout = 20 * time.Second
	gitFileListLimit   = 8 << 20
	gitSourceFileLimit = 256 << 10
	gitSourceRunLimit  = 320 << 10
	gitSourceTimeout   = 3 * time.Second
)

func initProjectGit(dir string) error {
	if _, err := os.Lstat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitSnapshotTimeout)
	defer cancel()
	_, err := workspaceGit(ctx, dir, nil, 0, "init", "--quiet", "--template=")
	return err
}

func workspaceGit(ctx context.Context, dir string, input []byte, limit int, args ...string) (*shared.CappedOutput, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{
		"-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null",
	}, args...)...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_OPTIONAL_LOCKS=0",
	}
	cmd.Stdin = bytes.NewReader(input)
	out := shared.NewCappedOutput(limit)
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out, nil
}

// The scratch repository and index let us compare working files even if a
// task stages, commits or changes branches in the project repository.
type taskGit struct {
	dir, scratch string
	root         *os.Root
}

func (g taskGit) git(ctx context.Context, input []byte, limit int, args ...string) (*shared.CappedOutput, error) {
	return workspaceGit(ctx, g.dir, input, limit, append([]string{
		"--git-dir=" + g.scratch, "--work-tree=.",
	}, args...)...)
}

func (g taskGit) snapshotDirectory(name string, directories map[string]bool) bool {
	if real, ok := directories[name]; ok {
		return real
	}
	real := false
	if g.snapshotDirectory(path.Dir(name), directories) {
		info, err := g.root.Lstat(name)
		real = err == nil && info.IsDir()
	}
	directories[name] = real
	return real
}

func (g taskGit) collectFiles(ctx context.Context, prefix string, input *bytes.Buffer, directories map[string]bool) error {
	paths, err := workspaceGit(ctx, filepath.Join(g.dir, prefix), nil, gitFileListLimit,
		"--git-dir=.git", "--work-tree=.",
		"ls-files", "--cached", "--others", "--exclude-standard", "--deduplicate", "-z")
	if err != nil {
		return err
	}
	if paths.Overflow {
		return errors.New("project file list exceeds the snapshot limit")
	}
	for entry := range strings.SplitSeq(string(paths.Data), "\x00") {
		name := prefix + strings.TrimSuffix(entry, "/")
		if !utf8.ValidString(name) || !shared.ValidProjectFile(name) || name == "work-requests" || strings.HasPrefix(name, "work-requests/") {
			continue
		}
		// The real index may still reference files beneath a directory that
		// the task replaced with a symbolic link.
		if !g.snapshotDirectory(path.Dir(name), directories) {
			continue
		}
		info, err := g.root.Lstat(name)
		if err != nil {
			continue
		}
		if info.IsDir() {
			// Git lists embedded repositories as one directory.
			if _, err := g.root.Lstat(name + "/.git"); err == nil {
				if err := g.collectFiles(ctx, name+"/", input, directories); err != nil {
					return err
				}
			}
			continue
		}
		if !(info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
			continue
		}
		if input.Len()+len(name)+1 > gitFileListLimit {
			return errors.New("project file list exceeds the snapshot limit")
		}
		input.WriteString(name)
		input.WriteByte(0)
	}
	return nil
}

func (g taskGit) snapshot(ctx context.Context) (string, error) {
	var input bytes.Buffer
	if err := g.collectFiles(ctx, "", &input, map[string]bool{".": true}); err != nil {
		return "", err
	}
	if _, err := g.git(ctx, nil, 0, "read-tree", "--empty"); err != nil {
		return "", err
	}
	if input.Len() > 0 {
		if _, err := g.git(ctx, input.Bytes(), 0, "update-index", "--add", "-z", "--stdin"); err != nil {
			return "", err
		}
	}
	tree, err := g.git(ctx, nil, 64, "write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(tree.Data)), nil
}

func (m *Manager) startTaskSnapshot(ctx context.Context) (taskGit, string, error) {
	scratch, err := filepath.Abs(filepath.Join(m.homePath(), ".workbench-task-git"))
	if err != nil {
		return taskGit{}, "", err
	}
	g := taskGit{dir: m.root.Name(), scratch: scratch, root: m.root}
	if err := os.RemoveAll(scratch); err != nil {
		return g, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, gitSnapshotTimeout)
	defer cancel()
	if _, err := workspaceGit(ctx, g.dir, nil, 0, "init", "--bare", "--quiet", "--template=", scratch); err != nil {
		return g, "", err
	}
	tree, err := g.snapshot(ctx)
	return g, tree, err
}

func (g taskGit) changes(before string) (*shared.TaskChanges, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitSnapshotTimeout)
	defer cancel()
	after, err := g.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	changes := &shared.TaskChanges{Files: []shared.TaskFileChange{}}
	if before == after {
		return changes, nil
	}
	flags := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames"}
	list, err := g.git(ctx, nil, 64<<10, append(flags, "--name-status", "-z", before, after, "--")...)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(list.Data), "\x00")
	for i := 0; i+2 < len(fields); i += 2 {
		changes.Files = append(changes.Files, shared.TaskFileChange{Status: fields[i], Path: fields[i+1]})
	}
	patch, err := g.git(ctx, nil, 192<<10, append(flags, "--unified=3", before, after, "--")...)
	if err != nil {
		changes.Truncated = true
		return changes, err
	}
	changes.Diff = strings.ToValidUTF8(string(patch.Data), "\uFFFD")
	changes.Truncated = list.Overflow || patch.Overflow
	g.captureSources(ctx, changes.Files, before, after)
	return changes, nil
}

// Store whole text blobs from the same trees as the patch. Later runs can
// overwrite the working tree, and a symlink blob contains only its link target.
func (g taskGit) captureSources(ctx context.Context, files []shared.TaskFileChange, before, after string) {
	ctx, cancel := context.WithTimeout(ctx, gitSourceTimeout)
	defer cancel()
	remaining := gitSourceRunLimit
	for i := range files {
		if ctx.Err() != nil || remaining == 0 {
			return
		}
		file := &files[i]
		tree := after
		if file.Status == "D" {
			tree = before
		}
		blob, err := g.git(ctx, nil, min(remaining, gitSourceFileLimit), "cat-file", "blob", tree+":"+file.Path)
		if err != nil || blob.Overflow || !utf8.Valid(blob.Data) || bytes.IndexByte(blob.Data, 0) != -1 {
			continue
		}
		// A non-nil slice encodes an empty file distinctly from an unavailable one.
		content := blob.Data
		if content == nil {
			content = []byte{}
		}
		file.Source = &content
		remaining -= len(content)
	}
}

func (m *Manager) taskChanges(w http.ResponseWriter, r *http.Request) {
	changes, err := m.tasks.Changes(r.Context(), r.PathValue("id"))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	if len(changes) == 0 {
		shared.Error(w, 404, "TASK_CHANGES_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, json.RawMessage(changes))
}

func (g taskGit) close() error {
	if g.scratch == "" {
		return nil
	}
	return os.RemoveAll(g.scratch)
}
