package shared

import "time"

type Schedule struct {
	CreatorID       string     `json:"creator_id,omitempty"`
	Revision        int        `json:"revision"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	ID              string     `json:"id"`
	ProjectID       string     `json:"project_id"`
	WorkspaceID     string     `json:"workspace_id"`
	Name            string     `json:"name"`
	InstructionPath string     `json:"instruction_path"`
	Provider        string     `json:"provider"`
	Enabled         bool       `json:"enabled"`
	RunAt           time.Time  `json:"run_at"`
	IntervalSeconds int64      `json:"interval_seconds"`
	NextRunAt       time.Time  `json:"next_run_at"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	LastErrorCode   string     `json:"last_error_code,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Run struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	WorkspaceID string    `json:"workspace_id"`
	ScheduleID  string    `json:"schedule_id"`
	TaskID      string    `json:"task_id,omitempty"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ErrorCode   string    `json:"error_code,omitempty"`
}
