package setup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var registryBackends = map[string]string{
	"bun": "core:bun", "go": "core:go", "node": "core:node",
	"pnpm": "aqua:pnpm/pnpm", "python": "core:python",
	"rust": "core:rust", "uv": "aqua:astral-sh/uv",
}

var toolInputs = append([]string{"mise.toml", ".mise.toml", "mise.lock", ".mise.lock"}, alternateConfigs...)

type toolEnvironment struct {
	Config string            `json:"config"`
	Tools  map[string]string `json:"tools"`
}

func toolDigest(config, lock []byte) string {
	h := sha256.New()
	_, _ = h.Write(config)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(lock)
	return hex.EncodeToString(h.Sum(nil))
}

func validateToolLock(content []byte) error {
	if content == nil {
		return nil
	}
	var lock struct {
		Tools map[string][]map[string]any `toml:"tools"`
	}
	if toml.Unmarshal(content, &lock) != nil || lock.Tools == nil {
		return ErrUnsupported
	}
	for name, entries := range lock.Tools {
		for _, entry := range entries {
			v, _ := entry["version"].(string)
			backend, _ := entry["backend"].(string)
			if !supportedTool(name) || backend != registryBackends[name] || !version.MatchString(v) {
				return ErrUnsupported
			}
			for field, value := range entry {
				switch field {
				case "version", "backend", "specifiers", "unsupported_platforms":
				default:
					fields, _ := value.(map[string]any)
					if !strings.HasPrefix(field, "platforms.") || !validToolArtifact(name, v, fields) {
						return ErrUnsupported
					}
				}
			}
		}
	}
	return nil
}

func validToolArtifact(name, version string, fields map[string]any) bool {
	if fields == nil {
		return false
	}
	for field, value := range fields {
		text, ok := value.(string)
		if !ok {
			return false
		}
		switch field {
		case "checksum", "checksum_source", "provenance":
		case "url", "url_api":
			if !matchesToolURL(name, version, field, text) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func matchesToolURL(name, version, field, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Path != path.Clean(u.Path) || strings.ContainsAny(u.Path, "\\\r\n") {
		return false
	}
	origin := u.Host + u.Path
	if field == "url_api" {
		for tool, prefix := range map[string]string{
			"pnpm": "api.github.com/repos/pnpm/pnpm/releases/assets/",
			"uv":   "api.github.com/repos/astral-sh/uv/releases/assets/",
		} {
			if name == tool && strings.HasPrefix(origin, prefix) {
				id := strings.TrimPrefix(origin, prefix)
				return id != "" && strings.Trim(id, "0123456789") == ""
			}
		}
		return false
	}
	switch name {
	case "node":
		return strings.HasPrefix(origin, "nodejs.org/dist/v"+version+"/node-v"+version+"-") ||
			strings.HasPrefix(origin, "unofficial-builds.nodejs.org/download/release/v"+version+"/node-v"+version+"-")
	case "go":
		return strings.HasPrefix(origin, "dl.google.com/go/go"+version+".") ||
			strings.HasPrefix(origin, "go.dev/dl/go"+version+".")
	case "bun":
		return strings.HasPrefix(origin, "github.com/oven-sh/bun/releases/download/bun-v"+version+"/bun-")
	case "pnpm":
		return strings.HasPrefix(origin, "github.com/pnpm/pnpm/releases/download/v"+version+"/pnpm-")
	case "python":
		asset := path.Base(u.Path)
		return strings.HasPrefix(origin, "github.com/astral-sh/python-build-standalone/releases/download/") &&
			strings.HasPrefix(asset, "cpython-"+version+"+")
	case "uv":
		return strings.HasPrefix(origin, "github.com/astral-sh/uv/releases/download/"+version+"/uv-")
	}
	return false
}

func (s Service) prepareTools(ctx context.Context, root *os.Root, p *Plan, req Request) error {
	snapshot, err := s.fingerprint(ctx, root, toolInputs)
	if err != nil {
		return err
	}
	p.snapshot = snapshot
	conf, err := parseTools(root)
	if err != nil {
		return err
	}
	if err := validateToolLock(conf.locked); err != nil {
		return err
	}
	if req.Operation == "tool-remove" && conf.tools[req.Name] == "" {
		return ErrChanged
	}
	active := filepath.Join(p.stage, "active")
	if os.MkdirAll(filepath.Join(active, "bin"), 0700) != nil {
		return ErrStorage
	}

	for name, content := range map[string][]byte{conf.name: conf.content, conf.lock: conf.locked} {
		if content != nil && os.WriteFile(filepath.Join(p.stage, name), content, 0644) != nil {
			return ErrStorage
		}
	}
	mise := func(args ...string) (string, error) {
		return s.command(ctx, p.stage, s.Mise, args...)
	}
	switch req.Operation {
	case "tool-set":
		if _, err := mise("use", "--path", conf.name, "--pin", req.Name+"@"+req.Version); err != nil {
			return err
		}
	case "tool-remove":
		if _, err := mise("use", "--path", conf.name, "--remove", req.Name); err != nil {
			return err
		}
	}
	empty := req.Operation == "tool-remove" && len(conf.tools) == 1 ||
		req.Operation == "tools-restore" && len(conf.tools) == 0
	if empty {
		if err := os.Remove(filepath.Join(p.stage, conf.lock)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return ErrStorage
		}
	} else {
		if req.Operation != "tools-restore" || conf.locked == nil {
			if _, err := mise("lock"); err != nil {
				return err
			}
		}
		if _, err := mise("install", "--locked"); err != nil {
			return err
		}
	}
	lockedMise := func(args ...string) (string, error) {
		return s.command(ctx, p.stage, s.Mise, append([]string{"--locked"}, args...)...)
	}
	// An empty toolset needs no lockfile.
	if empty {
		lockedMise = mise
	}
	resolved, err := resolveTools(lockedMise)
	if err != nil {
		return err
	}
	switch req.Operation {
	case "tool-set":
		if resolved[req.Name] == "" {
			return ErrDownload
		}
		p.Changes = append(p.Changes, fmt.Sprintf("Use %s %s on this worker.", req.Name, resolved[req.Name]))
	case "tool-remove":
		p.Changes = append(p.Changes, fmt.Sprintf("Remove %s from the workspace tool environment.", req.Name))
	default:
		if empty {
			p.Changes = append(p.Changes, "Use the worker’s bundled tools.")
		} else {
			p.Changes = append(p.Changes, "Activate the versions resolved by mise on this worker.")
		}
	}
	content := map[string][]byte{}
	for _, item := range []struct {
		name string
		old  []byte
	}{{conf.name, conf.content}, {conf.lock, conf.locked}} {
		data, err := os.ReadFile(filepath.Join(p.stage, item.name))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return ErrStorage
		}
		content[item.name] = data
		if !bytes.Equal(item.old, data) {
			p.files = append(p.files, item.name)
			text := "Update "
			if data == nil {
				text = "Remove "
			}
			p.Changes = append(p.Changes, text+item.name+".")
		}
	}
	env := toolEnvironment{Config: toolDigest(content[conf.name], content[conf.lock]), Tools: resolved}
	return s.stageToolPath(active, env, lockedMise)
}

func resolveTools(mise func(...string) (string, error)) (map[string]string, error) {
	raw, err := mise("ls", "--current", "--json")
	if err != nil {
		return nil, err
	}
	var records map[string][]struct {
		Version   string `json:"version"`
		Installed bool   `json:"installed"`
	}
	if json.Unmarshal([]byte(raw), &records) != nil {
		return nil, ErrDownload
	}
	resolved := map[string]string{}
	for name, versions := range records {
		if !supportedTool(name) || len(versions) != 1 || !versions[0].Installed {
			return nil, ErrDownload
		}
		resolved[name] = versions[0].Version
	}
	return resolved, nil
}

type rustEnvironment struct {
	CargoHome  string `json:"CARGO_HOME"`
	RustupHome string `json:"RUSTUP_HOME"`
	Toolchain  string `json:"RUSTUP_TOOLCHAIN"`
}

func (r rustEnvironment) wrapper(command string) []byte {
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
	}
	return fmt.Appendf(nil, "#!/bin/sh\nexport CARGO_HOME=%s RUSTUP_HOME=%s RUSTUP_TOOLCHAIN=%s\nexec %s \"$@\"\n",
		quote(r.CargoHome), quote(r.RustupHome), quote(r.Toolchain), quote(command))
}

func readRustEnvironment(state, version string, mise func(...string) (string, error)) (rustEnvironment, error) {
	var result rustEnvironment
	if version == "" {
		return result, nil
	}
	output, err := mise("env", "--json")
	if err != nil {
		return result, err
	}
	if json.Unmarshal([]byte(output), &result) != nil || result.Toolchain != version {
		return result, ErrUnsupported
	}
	for _, dir := range []string{result.RustupHome, result.CargoHome} {
		rel, err := filepath.Rel(state, dir)
		if err != nil || !filepath.IsLocal(rel) || rel == "." {
			return result, ErrUnsupported
		}
	}
	return result, nil
}

func (s Service) stageToolPath(active string, env toolEnvironment, mise func(...string) (string, error)) error {
	state := Dir(s.Home)
	rust, err := readRustEnvironment(state, env.Tools["rust"], mise)
	if err != nil {
		return err
	}
	paths, err := mise("bin-paths")
	if err != nil {
		return err
	}
	for _, dir := range strings.Split(strings.TrimSpace(paths), "\n") {
		if dir == "" {
			continue
		}
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return ErrDownload
		}
		rel, err := filepath.Rel(state, real)
		if err != nil || !filepath.IsLocal(rel) {
			return ErrUnsupported
		}
		entries, err := os.ReadDir(real)
		if err != nil {
			return ErrDownload
		}
		for _, entry := range entries {
			from := filepath.Join(real, entry.Name())
			info, err := os.Stat(from)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
				continue
			}
			to := filepath.Join(active, "bin", entry.Name())
			err = os.Symlink(from, to)
			if rust.Toolchain != "" && dir == filepath.Join(rust.CargoHome, "bin") && err == nil {
				// Rustup selects a toolchain from environment variables as well
				// as PATH. Pin it per invocation so staging a newer Rust
				// version cannot change the active version before apply.
				if os.Remove(to) != nil {
					return ErrStorage
				}
				err = os.WriteFile(to, rust.wrapper(from), 0700)
			}
			if err != nil && !errors.Is(err, fs.ErrExist) {
				return ErrStorage
			}
		}
	}
	b, _ := json.Marshal(env)
	if os.WriteFile(filepath.Join(active, "versions.json"), b, 0600) != nil {
		return ErrStorage
	}
	return nil
}

// Skill repositories use a single canonical HTTPS origin, including during restore.
func githubSource(source string) bool {
	u, err := url.Parse("https://github.com/" + source)
	if err != nil || u.Host != "github.com" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	_, repo, _ := strings.Cut(source, "/")
	return repoName.MatchString(source) && repo != "." && repo != ".." && !strings.HasPrefix(repo, "-")
}
