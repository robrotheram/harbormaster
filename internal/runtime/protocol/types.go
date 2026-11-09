package protocol

import (
	"agent-workbench/internal/shared"
	"context"
)

type Runtime interface {
	Start(context.Context, string) (shared.Observation, error)
	Stop(context.Context, string) error
	Inspect(context.Context, string) (shared.Observation, error)
	Agent(context.Context, string, string, string, []byte) (int, []byte, error)
}

type WorkspaceSpec struct {
	Resources      *ResourceLimits      `json:"resources,omitempty"`
	MCPs           []string             `json:"mcps"`
	HermesMatrix   *shared.HermesMatrix `json:"hermes_matrix,omitempty"`
	HermesModel    string               `json:"hermes_model,omitempty"`
	ConnectionMode string               `json:"connection_mode,omitempty"`
	ProjectID      string               `json:"project_id,omitempty"`
	HomeID         string               `json:"home_id,omitempty"`
	AgentKind      string               `json:"agent_kind,omitempty"`
	TemplateID     string               `json:"template_id,omitempty"`
	ProfileID      string               `json:"profile_id,omitempty"`
	Name           string               `json:"name,omitempty"`
	ParentID       string               `json:"parent_id,omitempty"`
	Branch         string               `json:"branch,omitempty"`
}
type RuntimeCapability struct {
	Resources       ResourceLimits `json:"resources"`
	SchemaVersion   int            `json:"schema_version"`
	Backend         string         `json:"backend"`
	BranchSeed      bool           `json:"branch_seed"`
	NetworkModes    []string       `json:"network_modes"`
	ID              string         `json:"id"`
	Version         string         `json:"version"`
	AgentKinds      []string       `json:"agent_kinds"`
	IsolationStatus string         `json:"isolation_status"`
	NetworkStatus   string         `json:"network_status"`
	PrivateHomes    bool           `json:"private_homes"`
}

// ResourceLimits describes the administrator-approved template, not arbitrary
// provider flags. The control plane reserves these same limits before launch.
type ResourceLimits struct {
	CPUCores    float64 `json:"cpu_cores"`
	MemoryBytes int64   `json:"memory_bytes"`
}
