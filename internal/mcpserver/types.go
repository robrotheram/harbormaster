// Package mcpserver exposes the local single-principal control API through MCP.
// Task submission and result reading are separate tools so future authorization
// can distinguish execution permission from access to potentially sensitive output.
package mcpserver

const MaxInstructionBytes = 65536
const MaxResultBytes = 65536

type Workspace struct {
	HomeID           string   `json:"home_id,omitempty"`
	AgentKind        string   `json:"agent_kind,omitempty"`
	TemplateID       string   `json:"template_id,omitempty"`
	Name             string   `json:"name,omitempty"`
	ParentID         string   `json:"parent_id,omitempty"`
	Branch           string   `json:"branch,omitempty"`
	ID               string   `json:"id"`
	ProjectID        string   `json:"project_id"`
	State            string   `json:"state"`
	Ready            bool     `json:"ready"`
	ActiveRuns       int      `json:"active_runs"`
	BackgroundJobs   int      `json:"background_jobs"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
	IdleStopAt       *string  `json:"idle_stop_at"`
	RuntimeType      string   `json:"runtime_type"`
	IsolationStatus  string   `json:"isolation_status"`
	NetworkStatus    string   `json:"network_status"`
	CPULimit         float64  `json:"cpu_limit"`
	MemoryLimitBytes int64    `json:"memory_limit_bytes"`
	CPUCores         *float64 `json:"cpu_cores"`
	MemoryBytes      *int64   `json:"memory_bytes"`
	ObservedAt       *string  `json:"observed_at"`
}
type WorkspaceList struct {
	Items []Workspace `json:"items"`
}
type Task struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	State       string `json:"state"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	ErrorCode   string `json:"error_code,omitempty"`
}
type Result struct {
	TaskID     string `json:"task_id"`
	State      string `json:"state"`
	Content    string `json:"content"`
	NextOffset int64  `json:"next_offset"`
}
type Usage struct {
	CPUAllocated         float64 `json:"cpu_allocated"`
	MemoryAllocatedBytes int64   `json:"memory_allocated_bytes"`
	RunningWorkspaces    int     `json:"running_workspaces"`
	MaxRunningWorkspaces int     `json:"max_running_workspaces"`
	CPUQuota             float64 `json:"cpu_quota"`
	MemoryQuotaBytes     int64   `json:"memory_quota_bytes"`
}
type Empty struct{}
type WorkspaceInput struct {
	WorkspaceID string `json:"workspace_id"`
}
type CreateInput struct {
	ProjectID      string `json:"project_id,omitempty"`
	AgentKind      string `json:"agent_kind,omitempty"`
	Name           string `json:"name,omitempty"`
	ParentID       string `json:"parent_id,omitempty"`
	Branch         string `json:"branch,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}
type MutationInput struct {
	WorkspaceID    string `json:"workspace_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
type SubmitInput struct {
	Provider       string `json:"provider,omitempty" jsonschema:"native subscription agent: codex or claude; omitted uses the deployment default"`
	WorkspaceID    string `json:"workspace_id"`
	Instruction    string `json:"instruction"`
	IdempotencyKey string `json:"idempotency_key"`
}
type TaskInput struct {
	WorkspaceID string `json:"workspace_id"`
	TaskID      string `json:"task_id"`
}
type ResultInput struct {
	WorkspaceID string `json:"workspace_id"`
	TaskID      string `json:"task_id"`
	Offset      int64  `json:"offset,omitempty"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum bytes to read; defaults to 65536"`
}
type CancelInput struct {
	WorkspaceID    string `json:"workspace_id"`
	TaskID         string `json:"task_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
