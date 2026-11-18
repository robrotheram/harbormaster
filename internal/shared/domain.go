package shared

import "time"

type Project struct {
	Sample              bool      `json:"sample,omitempty"`
	DeploymentNamespace string    `json:"deployment_namespace,omitempty"`
	NamespaceReady      bool      `json:"namespace_ready,omitempty"`
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	CreatedAt           time.Time `json:"created_at"`
}

func ValidAgentKind(kind string) bool { return kind == "t3" || kind == "codex" || kind == "hermes" }
