package shared

import "time"

// WorkItem contains operational references only. Specifications remain private
// in the owning workspace; Digest identifies the immutable specification bytes.
type ControlHandoff struct {
	From  string    `json:"from"`
	To    string    `json:"to"`
	Actor string    `json:"actor"`
	At    time.Time `json:"at"`
}

const (
	WorkKindDataScience = "data-science"
	WorkKindDesktop     = "desktop"
)

func ValidWorkKind(kind string) bool {
	switch kind {
	case "software", "research", "digest", WorkKindDataScience, WorkKindDesktop:
		return true
	default:
		return false
	}
}

type WorkItem struct {
	ControllerID        string           `json:"controller_id,omitempty"`
	ReviewerID          string           `json:"reviewer_id,omitempty"`
	ControlHistory      []ControlHandoff `json:"control_history,omitempty"`
	Provider            string           `json:"provider,omitempty"`
	Model               string           `json:"model,omitempty"`
	ReasoningEffort     string           `json:"reasoning_effort,omitempty"`
	ArtifactPaths       []string         `json:"artifact_paths,omitempty"`
	LatestAttemptID     string           `json:"latest_attempt_id,omitempty"`
	ID                  string           `json:"id"`
	ProjectID           string           `json:"project_id"`
	WorkspaceID         string           `json:"workspace_id"`
	SpecificationPath   string           `json:"specification_path"`
	SpecificationDigest string           `json:"specification_digest"`
	Kind                string           `json:"kind"`
	State               string           `json:"state"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

// WorkAttempt stores execution and review metadata; artifact bytes remain in the workspace.
type WorkAttempt struct {
	ReviewFeedback      string         `json:"review_feedback,omitempty"`
	ReviewedBy          string         `json:"reviewed_by,omitempty"`
	Provider            string         `json:"provider,omitempty"`
	Model               string         `json:"model,omitempty"`
	ReasoningEffort     string         `json:"reasoning_effort,omitempty"`
	SelectionRecorded   bool           `json:"selection_recorded,omitempty"`
	SpecificationPath   string         `json:"specification_path,omitempty"`
	SpecificationDigest string         `json:"specification_digest,omitempty"`
	ParentAttemptID     string         `json:"parent_attempt_id,omitempty"`
	ReviewState         string         `json:"review_state,omitempty"`
	DispatchStarted     bool           `json:"dispatch_started,omitempty"`
	ID                  string         `json:"id"`
	WorkItemID          string         `json:"work_item_id"`
	WorkspaceID         string         `json:"workspace_id"`
	TaskID              string         `json:"task_id"`
	State               string         `json:"state"`
	ErrorCode           string         `json:"error_code"`
	Artifacts           []WorkArtifact `json:"artifacts"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
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
