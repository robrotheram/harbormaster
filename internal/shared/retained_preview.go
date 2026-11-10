package shared

// RetainedPreview binds a separately allocated workspace to immutable published
// output. Only operational references enter platform metadata; bytes stay private.
type RetainedPreview struct {
	ServiceID         string `json:"service_id"`
	SourceWorkspaceID string `json:"source_workspace_id"`
	ArtifactID        string `json:"artifact_id"`
	ExpiresAt         int64  `json:"expires_at"`
}
