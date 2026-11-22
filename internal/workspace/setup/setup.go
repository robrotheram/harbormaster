// Package setup stages workspace tool and skill changes before publishing them.
package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const BasePath = "/usr/local/bin:/usr/bin:/bin"

var (
	ErrUnavailable = errors.New("SETUP_UNAVAILABLE")
	ErrInvalid     = errors.New("INVALID_REQUEST")
	ErrUnsupported = errors.New("SETUP_UNSUPPORTED_CONFIG")
	ErrChanged     = errors.New("SETUP_CHECKOUT_CHANGED")
	ErrConflict    = errors.New("SETUP_SKILL_CONFLICT")
	ErrNetwork     = errors.New("SETUP_NETWORK_BLOCKED")
	ErrDownload    = errors.New("SETUP_DOWNLOAD_FAILED")
	ErrStorage     = errors.New("SETUP_STORAGE_FAILED")
	ErrInterrupted = errors.New("SETUP_INTERRUPTED")
)

var (
	skillName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
	repoName  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,38}/[a-zA-Z0-9_.-]{1,100}$`)
	revision  = regexp.MustCompile(`^[a-f0-9]{40}$`)
	version   = regexp.MustCompile(`^[0-9]{1,5}(\.[0-9]{1,6}){0,2}([+-][0-9A-Za-z][0-9A-Za-z.-]{0,35})?$`)
)

var Catalog = []string{"bun", "go", "node", "pnpm", "python", "rust", "uv"}

type Request struct {
	Operation string `json:"operation"`
	Name      string `json:"name,omitempty"`
	Version   string `json:"version,omitempty"`
	Source    string `json:"source,omitempty"`
	Commit    string `json:"commit,omitempty"`
	PlanID    string `json:"plan_id,omitempty"`
}

type Tool struct {
	Name      string `json:"name"`
	Requested string `json:"requested"`
	Installed string `json:"installed,omitempty"`
}

type Skill struct {
	Name   string `json:"name"`
	Source string `json:"source,omitempty"`
	Commit string `json:"commit,omitempty"`
	State  string `json:"state"`
}

type Inventory struct {
	Branch        string   `json:"branch,omitempty"`
	ToolConfig    string   `json:"tool_config,omitempty"`
	ToolLock      string   `json:"tool_lock,omitempty"`
	ToolsNeedSync bool     `json:"tools_need_sync"`
	ToolIssue     string   `json:"tool_issue,omitempty"`
	SkillLock     bool     `json:"skill_lock"`
	SkillIssue    string   `json:"skill_issue,omitempty"`
	Tools         []Tool   `json:"tools"`
	Skills        []Skill  `json:"skills"`
	Catalog       []string `json:"catalog"`
	SkillsTarget  string   `json:"skills_target,omitempty"`
}

type Review struct {
	Changes []string `json:"changes"`
	Preview string   `json:"preview,omitempty"`
}

type Discovery struct {
	Source string   `json:"source"`
	Commit string   `json:"commit"`
	Names  []string `json:"names"`
}

type Service struct {
	Data, Home string
	Proxy      bool
	Blocked    bool
	Skills     bool
	Mise, CLI  string
}

func (s Service) Available() bool {
	info, err := os.Stat(s.Mise)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

func Dir(home string) string { return filepath.Join(home, "workspace-setup") }

// Path adds an atomic symlink to the last applied tool environment. Processes
// started before setup, including T3, pick up the new commands at invocation.
func Path(home string) string {
	return filepath.Join(Dir(home), "current", "bin") + ":" + BasePath
}

func (s Service) project() string { return filepath.Join(s.Data, "files") }

func (s Service) Validate(req *Request) error {
	switch req.Operation {
	case "tool-set":
		if !supportedTool(req.Name) || !supportedRequest(req.Version) {
			return ErrInvalid
		}
	case "tool-remove":
		if !supportedTool(req.Name) {
			return ErrInvalid
		}
	case "tools-restore":
	case "skill-discover", "skill-add":
		req.Source = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(req.Source), "https://github.com/"), ".git")
		if !s.Skills || !githubSource(req.Source) {
			return ErrInvalid
		}
		if req.Operation == "skill-add" && (!skillName.MatchString(req.Name) || !revision.MatchString(req.Commit)) {
			return ErrInvalid
		}
	case "skill-remove":
		if !s.Skills || !skillName.MatchString(req.Name) {
			return ErrInvalid
		}
	case "skills-restore":
		if !s.Skills {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

type Plan struct {
	Review
	stage      string
	snapshot   string
	operation  string
	skillNames []string
	files      []string
}

func (s Service) Prepare(ctx context.Context, id string, req Request) (*Plan, error) {
	if err := s.Validate(&req); err != nil || req.Operation == "skill-discover" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(Dir(s.Home), 0700); err != nil {
		return nil, ErrStorage
	}
	stage, err := os.MkdirTemp(Dir(s.Home), "plan-"+id+"-")
	if err != nil {
		return nil, ErrStorage
	}
	p := &Plan{stage: stage, operation: req.Operation}
	if err := s.prepare(ctx, p, req); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func (p *Plan) Close() {
	// A failed rollback still needs the journal and backups on the next boot.
	if _, err := os.Lstat(filepath.Join(p.stage, "journal.json")); err == nil {
		if _, err := os.Lstat(filepath.Join(p.stage, "committed")); err != nil {
			return
		}
	}
	_ = os.RemoveAll(p.stage)
}

func (s Service) prepare(ctx context.Context, p *Plan, req Request) error {
	root, err := os.OpenRoot(s.project())
	if err != nil {
		return ErrStorage
	}
	defer root.Close()
	if strings.HasPrefix(req.Operation, "tool") {
		return s.prepareTools(ctx, root, p, req)
	}
	return s.prepareSkills(ctx, root, p, req)
}

func supportedRequest(request string) bool {
	return request == "latest" || version.MatchString(request)
}

func supportedTool(name string) bool {
	for _, tool := range Catalog {
		if name == tool {
			return true
		}
	}
	return false
}
