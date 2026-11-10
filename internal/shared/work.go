package shared

import "time"

// WorkItem contains operational references only. Specifications remain private
// in the owning workspace; Digest identifies the immutable specification bytes.
type WorkItem struct {
	ArtifactPaths       []string  `json:"artifact_paths,omitempty"`
	LatestAttemptID     string    `json:"latest_attempt_id,omitempty"`
	ID                  string    `json:"id"`
	ProjectID           string    `json:"project_id"`
	WorkspaceID         string    `json:"workspace_id"`
	SpecificationPath   string    `json:"specification_path"`
	SpecificationDigest string    `json:"specification_digest"`
	Kind                string    `json:"kind"`
	State               string    `json:"state"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// WorkAttempt stores references only; artifact bytes remain in the workspace.
type WorkAttempt struct {
	DispatchStarted bool           `json:"dispatch_started,omitempty"`
	ID              string         `json:"id"`
	WorkItemID      string         `json:"work_item_id"`
	WorkspaceID     string         `json:"workspace_id"`
	TaskID          string         `json:"task_id"`
	State           string         `json:"state"`
	ErrorCode       string         `json:"error_code"`
	Artifacts       []WorkArtifact `json:"artifacts"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}
type WorkArtifact struct {
	ID          string    `json:"id"`
	SourcePath  string    `json:"source_path,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	ContentURL  string    `json:"content_url"`
	SHA256      string    `json:"sha256,omitempty"`
	SizeBytes   int64     `json:"size_bytes,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
}
