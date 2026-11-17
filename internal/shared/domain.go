package shared

import "time"

type Project struct {
	Sample              bool      `json:"sample,omitempty"`
	DeploymentNamespace string    `json:"deployment_namespace,omitempty"`
	NamespaceReady      bool      `json:"namespace_ready,omitempty"`
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	CreatedAt           time.Time `json:"created_at"`
	Legacy              bool      `json:"legacy,omitempty"`
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
