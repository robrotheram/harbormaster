package shared

import (
	"path"
	"strings"
	"unicode/utf8"
)

const HandoffLimit = 1 << 20
const ReportLimit = 256 << 10

type ReportWrite struct {
	Path     string `json:"path"`
	Markdown string `json:"markdown"`
	// A nil Previous creates a file. An edit must match the last read revision.
	Previous *string `json:"previous,omitempty"`
}

func (r ReportWrite) Valid() bool {
	if !ValidProjectFile(r.Path) || !utf8.ValidString(r.Path) || strings.HasPrefix(r.Path, "work-requests/") {
		return false
	}
	ext := strings.ToLower(path.Ext(r.Path))
	if ext != ".md" && ext != ".markdown" {
		return false
	}
	if r.Previous == nil {
		if path.Dir(r.Path) != "reports" {
			return false
		}
	} else if len(*r.Previous) > ReportLimit {
		return false
	}
	return strings.TrimSpace(r.Markdown) != "" && len(r.Markdown) <= ReportLimit && !strings.ContainsRune(r.Markdown, 0)
}

type FileHandoff struct {
	Path    string `json:"path"`
	Content []byte `json:"content,omitempty"`
}

type WorkspaceFiles struct {
	Paths     []string `json:"paths"`
	Truncated bool     `json:"truncated,omitempty"`
}

func ValidProjectFile(value string) bool {
	if value == "" || len(value) > 512 || path.IsAbs(value) || path.Clean(value) != value || strings.ContainsAny(value, "\\\x00\r\n") {
		return false
	}
	for _, p := range strings.Split(value, "/") {
		if p == "." || p == ".." || p == ".git" || p == ".t3" || p == ".codex" || p == ".claude" || p == ".env" || (strings.HasPrefix(p, ".env.") && p != ".env.example" && p != ".env.sample") {
			return false
		}
	}
	return true
}
