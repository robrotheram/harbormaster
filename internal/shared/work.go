package shared

import "time"

// WorkItem contains operational references only. Specifications remain private
// in the owning workspace; Digest identifies the immutable specification bytes.
type WorkItem struct {
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
