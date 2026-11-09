package shared

import (
	"regexp"
	"time"
)

var branchPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9/_-]{0,79}$`)

func ValidBranch(value string) bool { return branchPattern.MatchString(value) }

type Profile struct {
	Revision    int       `json:"revision"`
	UpdatedAt   time.Time `json:"updated_at"`
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	MCPs        []string  `json:"mcps"`
}

// MVP profiles select baked-in local MCP servers only. Remote connectors require
// separate administrator policy/credential configuration; no URL is accepted.
func Profiles() []Profile {
	return []Profile{
		{ID: "general", Title: "Development", Description: "Code, tests and Git in a persistent project workspace.", MCPs: []string{"workspace-files", "project-git"}},
		{ID: "research", Title: "Vulnerability research", Description: "A separate research workspace. External targets need an approved network policy.", MCPs: []string{"workspace-files", "project-git"}},
		{ID: "banking", Title: "Banking project", Description: "A separate project workspace. No bank account or transaction connector is configured.", MCPs: []string{"workspace-files", "project-git"}},
		{ID: "gallery", Title: "Photo gallery", Description: "Build a gallery application with its own files, threads and branch workspaces.", MCPs: []string{"workspace-files", "project-git"}},
	}
}
func ValidMCPs(names []string) bool {
	seen := map[string]bool{}
	for _, name := range names {
		if (name != "workspace-files" && name != "project-git") || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}
