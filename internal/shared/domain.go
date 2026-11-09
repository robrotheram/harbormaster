package shared

import "time"

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Legacy    bool      `json:"legacy,omitempty"`
}

type AgentHome struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	WorkspaceID string    `json:"workspace_id"`
	Kind        string    `json:"kind"`
	CreatedAt   time.Time `json:"created_at"`
}

func ValidAgentKind(kind string) bool { return kind == "t3" || kind == "codex" || kind == "hermes" }

// Legacy workspaces retain their existing private content on the existing
// runtime volume. An empty legacy HomeID is intentionally preserved so runtime
// startup cannot move or hide native history by assuming a new storage layout.
func NormalizeWorkspace(w Workspace) Workspace {
	if w.ProjectID == "" {
		w.ProjectID = "default"
	}
	if w.AgentKind == "" {
		if w.ProfileID != "" {
			w.AgentKind = "t3"
		} else {
			w.AgentKind = "codex"
		}
	}
	return w
}
