package shared

// AgentModel is a runnable selection, not an upstream credential or endpoint.
type AgentModel struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}
type AgentModels struct {
	WorkspaceID      string              `json:"workspace_id,omitempty"`
	DefaultProvider  string              `json:"default_provider,omitempty"`
	DefaultModel     string              `json:"default_model,omitempty"`
	Items            []AgentModel        `json:"items"`
	ReasoningEfforts map[string][]string `json:"reasoning_efforts,omitempty"`
}
