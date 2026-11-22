package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	skillDir  = ".agents/skills"
	skillLock = "skills-lock.json"
	metaLimit = 512 << 10
)

var alternateConfigs = []string{
	".tool-versions", "mise.local.toml", ".mise.local.toml", ".mise/config.toml",
	".config/mise.toml", ".config/mise/config.toml", ".config/mise/conf.d",
	"mise/conf.d", ".mise/conf.d",
}

type toolConfig struct {
	name, lock      string
	content, locked []byte
	tools           map[string]string
}

type lockedSkill struct {
	Source     string `json:"source"`
	SourceType string `json:"sourceType"`
	Ref        string `json:"ref"`
	Commit     string `json:"commit"`
	Hash       string `json:"computedHash"`
}

type skillLockfile struct {
	Version int
	Skills  map[string]json.RawMessage
	Fields  map[string]json.RawMessage
}

func readFile(root *os.Root, name string, limit int64) ([]byte, error) {
	info, err := cleanStat(root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, ErrUnsupported
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, ErrStorage
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, ErrStorage
	}
	return b, nil
}

// Reject symlinks even when they happen to point elsewhere within the checkout.
func cleanStat(root *os.Root, name string) (fs.FileInfo, error) {
	prefix := ""
	var info fs.FileInfo
	for _, part := range strings.Split(name, "/") {
		prefix = path.Join(prefix, part)
		var err error
		info, err = root.Lstat(prefix)
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrUnsupported
		}
	}
	return info, nil
}

func parseTools(root *os.Root) (toolConfig, error) {
	conf := toolConfig{name: "mise.toml", lock: "mise.lock", tools: map[string]string{}}
	content, err := readFile(root, conf.name, metaLimit)
	if err != nil {
		return conf, err
	}
	dot, err := readFile(root, ".mise.toml", metaLimit)
	if err != nil {
		return conf, err
	}
	if dot != nil {
		conf.name = ".mise.toml"
		if content != nil {
			return conf, ErrUnsupported
		}
		content = dot
	}
	conf.content = content
	conf.locked, err = readFile(root, conf.lock, metaLimit)
	if err != nil {
		return conf, err
	}
	for _, name := range alternateConfigs {
		if _, err := root.Lstat(name); !errors.Is(err, fs.ErrNotExist) {
			return conf, ErrUnsupported
		}
	}
	var doc struct {
		Tools map[string]string `toml:"tools"`
	}
	if err := toml.NewDecoder(strings.NewReader(string(content))).DisallowUnknownFields().Decode(&doc); err != nil {
		return conf, ErrUnsupported
	}
	for name, requested := range doc.Tools {
		if !supportedTool(name) || !supportedRequest(requested) {
			return conf, ErrUnsupported
		}
		conf.tools[name] = requested
	}
	if _, err := root.Lstat(".mise.lock"); !errors.Is(err, fs.ErrNotExist) {
		return conf, ErrUnsupported
	}
	return conf, nil
}

func parseSkillLock(raw []byte) (skillLockfile, error) {
	lock := skillLockfile{
		Version: 2, Skills: map[string]json.RawMessage{}, Fields: map[string]json.RawMessage{},
	}
	if raw == nil {
		return lock, nil
	}
	if json.Unmarshal(raw, &lock.Fields) != nil ||
		json.Unmarshal(lock.Fields["version"], &lock.Version) != nil ||
		json.Unmarshal(lock.Fields["skills"], &lock.Skills) != nil ||
		lock.Skills == nil || lock.Version < 1 || lock.Version > 2 || len(lock.Skills) > 256 {
		return lock, ErrUnsupported
	}
	return lock, nil
}

func lockEntry(raw json.RawMessage) lockedSkill {
	var entry lockedSkill
	_ = json.Unmarshal(raw, &entry)
	return entry
}

func (s Service) branch(ctx context.Context) (string, string) {
	name, _ := s.command(ctx, s.project(), "/usr/bin/git", "symbolic-ref", "--quiet", "--short", "HEAD")
	head, _ := s.command(ctx, s.project(), "/usr/bin/git", "rev-parse", "--verify", "HEAD")
	return strings.TrimSpace(name), strings.TrimSpace(head)
}

func (s Service) Inventory(ctx context.Context) (Inventory, error) {
	out := Inventory{
		Tools: []Tool{}, Skills: []Skill{}, Catalog: Catalog,
	}
	root, err := os.OpenRoot(s.project())
	if err != nil {
		return out, ErrStorage
	}
	defer root.Close()
	out.Branch, _ = s.branch(ctx)
	conf, err := parseTools(root)
	out.ToolConfig, out.ToolLock = conf.name, conf.lock
	if err == nil {
		err = validateToolLock(conf.locked)
	}
	if err != nil {
		out.ToolIssue = err.Error()
	} else {
		var active toolEnvironment
		if b, err := os.ReadFile(filepath.Join(Dir(s.Home), "current", "versions.json")); err == nil {
			_ = json.Unmarshal(b, &active)
		}
		out.ToolsNeedSync = (len(active.Tools) > 0 || len(conf.tools) > 0) &&
			active.Config != toolDigest(conf.content, conf.locked)
		for name, requested := range conf.tools {
			out.Tools = append(out.Tools, Tool{Name: name, Requested: requested, Installed: active.Tools[name]})
		}
		slices.SortFunc(out.Tools, func(a, b Tool) int { return strings.Compare(a.Name, b.Name) })
	}
	if s.Skills {
		out.SkillsTarget = "Codex"
		out.Skills, out.SkillLock, err = skillInventory(root)
		if err != nil {
			out.SkillIssue = err.Error()
		}
	}
	return out, nil
}

func skillInventory(root *os.Root) ([]Skill, bool, error) {
	list := []Skill{}
	raw, err := readFile(root, skillLock, metaLimit)
	if err != nil {
		return list, false, err
	}
	lock, err := parseSkillLock(raw)
	if err != nil {
		return list, false, err
	}
	for name, value := range lock.Skills {
		if !skillName.MatchString(name) {
			continue
		}
		entry := lockEntry(value)
		item := Skill{Name: name, Source: entry.Source, Commit: entry.Commit, State: "missing"}
		if info, err := cleanStat(root, path.Join(skillDir, name, "SKILL.md")); err == nil && info.Mode().IsRegular() {
			item.State = "installed"
		}
		list = append(list, item)
	}
	if _, err := cleanStat(root, skillDir); err == nil {
		items, err := fs.ReadDir(root.FS(), skillDir)
		if err != nil {
			return list, raw != nil, ErrStorage
		}
		for _, item := range items {
			if _, tracked := lock.Skills[item.Name()]; tracked || !item.IsDir() || len(list) >= 256 {
				continue
			}
			if info, err := cleanStat(root, path.Join(skillDir, item.Name(), "SKILL.md")); err == nil && info.Mode().IsRegular() {
				list = append(list, Skill{Name: item.Name(), State: "local"})
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return list, raw != nil, ErrUnsupported
	}
	slices.SortFunc(list, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return list, raw != nil, nil
}

func (s Service) fingerprint(ctx context.Context, root *os.Root, paths []string) (string, error) {
	branch, head := s.branch(ctx)
	if ctx.Err() != nil {
		return "", ErrInterrupted
	}
	h := sha256.New()
	_, _ = io.WriteString(h, branch+"\x00"+head+"\x00")
	sorted := slices.Clone(paths)
	sort.Strings(sorted)
	for _, name := range sorted {
		digest, err := treeDigest(root, name)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(h, name+"\x00"+digest+"\x00")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func treeDigest(root *os.Root, name string) (string, error) {
	_, err := cleanStat(root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", ErrUnsupported
	}
	h := sha256.New()
	write := func(item string) { _, _ = io.WriteString(h, item+"\x00") }
	var files int
	var size int64
	err = fs.WalkDir(root.FS(), name, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		write(file)
		if d.IsDir() {
			write("directory")
			return nil
		}
		files++
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() || files > 2048 || size+info.Size() > 24<<20 {
			return ErrUnsupported
		}
		size += info.Size()
		f, err := root.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		digest := sha256.New()
		n, err := io.Copy(digest, io.LimitReader(f, info.Size()+1))
		if err != nil || n != info.Size() {
			return ErrChanged
		}
		write(hex.EncodeToString(digest.Sum(nil)))
		return nil
	})
	if err != nil {
		return "", ErrUnsupported
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
