package shared

// AgentModel is a runnable selection, not an upstream credential or endpoint.
type AgentModel struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}
type AgentModels struct {
	WorkspaceID     string       `json:"workspace_id,omitempty"`
	DefaultProvider string       `json:"default_provider,omitempty"`
	DefaultModel    string       `json:"default_model,omitempty"`
	Items           []AgentModel `json:"items"`
}
