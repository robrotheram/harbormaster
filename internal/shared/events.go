package shared

import "time"

// ActivityEvent is an allowlisted progress record, never a raw agent log line.
type ActivityEvent struct {
	ID     string    `json:"id"`
	At     time.Time `json:"at"`
	Source string    `json:"source"`
	Kind   string    `json:"kind"`
	TaskID string    `json:"task_id,omitempty"`
	Tool   string    `json:"tool,omitempty"`
	Status string    `json:"status,omitempty"`
}

func SafeActivityEvent(e ActivityEvent) bool {
	if !ValidID(e.ID) || e.At.IsZero() || e.At.After(time.Now().Add(time.Minute)) {
		return false
	}
	switch e.Source {
	case "workspace", "codex", "claude", "hermes", "matrix", "t3":
	default:
		return false
	}
	switch e.Kind {
	case "task.queued", "task.started", "task.succeeded", "task.failed", "task.cancelled", "tool.started", "tool.completed", "agent.started", "agent.completed", "agent.failed", "service.started", "service.connected", "service.stopped", "service.failed", "message.received", "response.started", "response.completed", "response.failed", "workspace.state", "agent.state":
	default:
		return false
	}
	if e.TaskID != "" && !ValidID(e.TaskID) {
		return false
	}
	switch e.Tool {
	case "", "terminal", "file", "memory", "web", "mcp", "other":
	default:
		return false
	}
	if e.Kind == "agent.failed" && e.Status != "" && SanitizedTaskError(e.Status) == e.Status {
		return true
	}
	switch e.Status {
	case "", "STOPPED", "STARTING", "READY", "BUSY", "UNKNOWN", "ERROR", "idle", "running", "waiting", "failed", "connected", "disconnected", "starting", "queued", "cancelled", "succeeded":
	default:
		return false
	}
	return true
}
func ActivityTool(name string) string {
	switch name {
	case "terminal", "shell", "exec_command", "command_execution", "bash", "Bash", "execute_command":
		return "terminal"
	case "file", "read_file", "write_file", "edit_file", "apply_patch", "file_change", "Read", "Write", "Edit", "Glob", "Grep":
		return "file"
	case "memory", "memory_search":
		return "memory"
	case "web_search", "web_search_call", "web_fetch", "WebSearch", "WebFetch":
		return "web"
	case "mcp_tool_call":
		return "mcp"
	default:
		return "other"
	}
}
