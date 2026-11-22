package setup

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var skillListLine = regexp.MustCompile(`(?m)^│    ([a-zA-Z0-9][a-zA-Z0-9_-]{0,63})\r?$`)

func (s Service) Discover(ctx context.Context, req Request) (*Discovery, error) {
	if err := s.Validate(&req); err != nil || req.Operation != "skill-discover" {
		return nil, ErrInvalid
	}
	base := Dir(s.Home)
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, ErrStorage
	}
	tmp, err := os.MkdirTemp(base, "discover-")
	if err != nil {
		return nil, ErrStorage
	}
	defer os.RemoveAll(tmp)
	refs, err := s.command(ctx, tmp, "/usr/bin/git", "ls-remote", "--exit-code", "https://github.com/"+req.Source+".git", "HEAD")
	line := strings.Fields(refs)
	if err != nil {
		return nil, err
	}
	if len(line) != 2 || line[1] != "HEAD" || !revision.MatchString(line[0]) {
		return nil, ErrDownload
	}
	output, err := s.command(ctx, tmp, s.CLI, "add", req.Source+"@"+line[0], "--list", "--agent", "codex", "--yes")
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, match := range skillListLine.FindAllStringSubmatch(output, 257) {
		names = append(names, match[1])
	}
	if len(names) > 256 {
		return nil, ErrUnsupported
	}
	slices.Sort(names)
	return &Discovery{Source: req.Source, Commit: line[0], Names: slices.Compact(names)}, nil
}

func (s Service) prepareSkills(ctx context.Context, root *os.Root, p *Plan, req Request) error {
	raw, err := readFile(root, skillLock, metaLimit)
	if err != nil {
		return err
	}
	lock, err := parseSkillLock(raw)
	if err != nil {
		return err
	}
	if req.Operation != "skill-remove" && lock.Version != 2 {
		return ErrUnsupported
	}
	if req.Operation == "skills-restore" {
		if len(lock.Skills) == 0 {
			return ErrInvalid
		}
		for name, rawEntry := range lock.Skills {
			entry := lockEntry(rawEntry)
			if !skillName.MatchString(name) || entry.SourceType != "github" || !githubSource(entry.Source) ||
				!revision.MatchString(entry.Ref) || entry.Ref != entry.Commit || !contentHash(entry.Hash) {
				return ErrUnsupported
			}
			p.skillNames = append(p.skillNames, name)
		}
		slices.Sort(p.skillNames)
	} else {
		p.skillNames = []string{req.Name}
		p.files = []string{path.Join(skillDir, req.Name), skillLock}
		_, tracked := lock.Skills[req.Name]
		info, err := cleanStat(root, path.Join(skillDir, req.Name))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return ErrConflict
		}
		if req.Operation == "skill-remove" && !tracked {
			return ErrConflict
		}
		if req.Operation == "skill-add" && (tracked || info != nil) {
			return ErrConflict
		}
	}
	fingerprint, err := s.fingerprint(ctx, root, p.inputs())
	if err != nil {
		return err
	}
	p.snapshot = fingerprint
	current, err := readFile(root, skillLock, metaLimit)
	if err != nil || !bytes.Equal(current, raw) {
		return ErrChanged
	}
	if req.Operation == "skill-remove" {
		delete(lock.Skills, req.Name)
		if err := saveSkillLock(p.stage, lock); err != nil {
			return err
		}
		p.Changes = []string{"Remove " + req.Name + " from Codex.", "Update " + skillLock + "."}
		return nil
	}
	if req.Operation == "skills-restore" {
		if err := os.WriteFile(filepath.Join(p.stage, skillLock), raw, 0644); err != nil {
			return ErrStorage
		}
		if _, err := s.command(ctx, p.stage, s.CLI, "experimental_install", "--frozen-lockfile", "--agent", "codex"); err != nil {
			return err
		}
		p.Changes = []string{"Restore Codex skills from " + skillLock + "."}
	} else {
		if _, err := s.command(ctx, p.stage, s.CLI, "add", req.Source+"@"+req.Commit, "--skill", req.Name, "--agent", "codex", "--yes", "--json", "--copy"); err != nil {
			return err
		}
		b, err := os.ReadFile(filepath.Join(p.stage, skillLock))
		if err != nil || len(b) > metaLimit {
			return ErrDownload
		}
		updated, err := parseSkillLock(b)
		if err != nil {
			return err
		}
		entry := lockEntry(updated.Skills[req.Name])
		if entry.Commit != req.Commit || entry.Ref != req.Commit || !strings.EqualFold(entry.Source, req.Source) {
			return ErrDownload
		}
		lock.Skills[req.Name] = updated.Skills[req.Name]
		if err := saveSkillLock(p.stage, lock); err != nil {
			return err
		}
		p.Changes = []string{"Install " + req.Name + " for Codex from " + req.Source + " at " + req.Commit + ".", "Update " + skillLock + "."}
	}
	stage, err := os.OpenRoot(p.stage)
	if err != nil {
		return ErrStorage
	}
	defer stage.Close()
	for _, name := range p.skillNames {
		location := path.Join(skillDir, name)
		file := path.Join(location, "SKILL.md")
		md, err := readFile(stage, file, 256<<10)
		if err != nil || len(md) == 0 {
			return ErrDownload
		}
		prepared, err := treeDigest(stage, location)
		if err != nil {
			return err
		}
		if req.Operation == "skill-add" {
			p.Preview = string(md)
			continue
		}
		installed, err := treeDigest(root, location)
		if err != nil || (installed != "" && installed != prepared) {
			return ErrConflict
		}
		if installed == "" {
			p.files = append(p.files, location)
		}
	}
	return nil
}

func contentHash(s string) bool {
	decoded, err := hex.DecodeString(s)
	return err == nil && len(decoded) == 32
}

func saveSkillLock(stage string, lock skillLockfile) error {
	skills, err := json.Marshal(lock.Skills)
	if err != nil {
		return ErrStorage
	}
	lock.Fields["version"] = json.RawMessage{byte('0' + lock.Version)}
	lock.Fields["skills"] = skills
	b, err := json.MarshalIndent(lock.Fields, "", "  ")
	if err != nil || len(b) >= metaLimit {
		return ErrStorage
	}
	if os.WriteFile(filepath.Join(stage, skillLock), append(b, '\n'), 0644) != nil {
		return ErrStorage
	}
	return nil
}

func (p *Plan) skillPaths() []string {
	items := make([]string, 0, len(p.skillNames))
	for _, name := range p.skillNames {
		items = append(items, path.Join(skillDir, name))
	}
	return items
}

func (p *Plan) inputs() []string {
	if strings.HasPrefix(p.operation, "tool") {
		return toolInputs
	}
	return append(p.skillPaths(), skillLock)
}
