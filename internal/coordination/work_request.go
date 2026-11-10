package coordination

// WorkRequest is explicitly selected export content. Instructions travel only
// through private blob storage and destination-authorized consumption.
type WorkRequest struct {
	Instruction   *string  `json:"instruction,omitempty"`
	WorkspaceID   string   `json:"workspace_id,omitempty"`
	Path          string   `json:"specification_path,omitempty"`
	Digest        string   `json:"specification_digest,omitempty"`
	Kind          string   `json:"kind"`
	ArtifactPaths []string `json:"artifact_paths,omitempty"`
}
