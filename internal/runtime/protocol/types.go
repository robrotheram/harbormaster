package protocol

import (
	"agent-workbench/internal/shared"
	"context"
	"net/http"
)

var ErrCapabilitiesIncompatible = &shared.CodedError{Status: shared.StatusServiceUnavailable, Code: "RUNTIME_CAPABILITIES_INCOMPATIBLE"}

// Runtime is the control plane contract with the runtime controller.
type Runtime interface {
	StartConfigured(context.Context, string, WorkspaceSpec) (shared.Observation, error)
	Stop(context.Context, string) error
	Inspect(context.Context, string) (shared.Observation, error)
	Agent(context.Context, string, string, string, []byte) (int, []byte, error)
	Reconcile(context.Context, string) error
	ConfigureConnections(context.Context, string, shared.ConnectionPolicy) error
	Capabilities(context.Context) (RuntimeCapability, error)
	MatrixCredentialPresent(context.Context, string) bool
	MatrixCredential(context.Context, string, shared.HermesMatrix, string) error
	MatrixAction(context.Context, string, string, *shared.HermesMatrix) error
	EnsureProjectNamespace(context.Context, string) (string, error)
	Desktop(http.ResponseWriter, *http.Request, string, string)
	Console(http.ResponseWriter, *http.Request, string, string)
	Application(http.ResponseWriter, *http.Request, string, string)
}

type WorkspaceSpec struct {
	ConsoleEnabled bool                 `json:"console_enabled,omitempty"`
	Resources      *ResourceLimits      `json:"resources,omitempty"`
	HermesMatrix   *shared.HermesMatrix `json:"hermes_matrix,omitempty"`
	HermesModel    string               `json:"hermes_model,omitempty"`
	ConnectionMode string               `json:"connection_mode,omitempty"`
	ProjectID      string               `json:"project_id,omitempty"`
	HomeID         string               `json:"home_id,omitempty"`
	AgentKind      string               `json:"agent_kind,omitempty"`
	TemplateID     string               `json:"template_id,omitempty"`
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

// DevelopmentCapability returns the common development template capabilities.
// Backends add optional agents and features after checking their configuration.
func DevelopmentCapability(backend, version string, memoryBytes int64) RuntimeCapability {
	return RuntimeCapability{
		SchemaVersion: 1, Backend: backend, Version: version, ID: DevelopmentTemplate,
		Resources:  ResourceLimits{CPUCores: 1, MemoryBytes: memoryBytes},
		AgentKinds: []string{"t3", "codex"}, NetworkModes: []string{"blocked", "allowlist"},
		IsolationStatus: "unverified", NetworkStatus: "unverified", PrivateHomes: true,
	}
}
