package shared

import (
	"path"
	"strings"
)

const HandoffLimit = 1 << 20

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
