package shared

type TaskChanges struct {
	Files     []TaskFileChange `json:"files"`
	Diff      string           `json:"diff,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
}

type TaskFileChange struct {
	Path   string  `json:"path"`
	Status string  `json:"status"`
	Source *[]byte `json:"source,omitempty"`
}

func ValidTask(t Task, workspace string) bool {
	if !ValidID(t.ID) || t.WorkspaceID != workspace {
		return false
	}
	switch t.State {
	case "queued", "running", "cancel_requested", "succeeded", "failed", "cancelled":
		return true
	}
	return false
}
func SanitizedTaskError(code string) string {
	if code == "" {
		return ""
	}
	switch code {
	case "AGENT_SELECTION_REQUIRED", "PROVIDER_NOT_CONFIGURED", "MODEL_NOT_ALLOWED", "PROVIDER_LOGIN_REQUIRED", "CREDENTIAL_UNAVAILABLE", "AGENT_EXECUTABLE_UNAVAILABLE", "NATIVE_AGENT_FAILED", "RESULT_UNAVAILABLE", "INVALID_AGENT_RESPONSE":
		return code
	case "INTERRUPTED", "INFERENCE_UNAVAILABLE", "INFERENCE_REJECTED", "INVALID_INFERENCE_RESPONSE", "TOOL_LIMIT", "RESULT_TOO_LARGE", "STORAGE_UNAVAILABLE", "TURN_LIMIT", "EXECUTION_INTERRUPTED":
		return code
	}
	return "EXECUTION_FAILED"
}
