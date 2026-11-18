package api

import (
	"encoding/json"
	"net/http"

	"agent-workbench/internal/shared"
)

func (s *Server) shareFile(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SourceID        string `json:"source_id"`
		SourcePath      string `json:"source_path"`
		DestinationID   string `json:"destination_id"`
		DestinationPath string `json:"destination_path"`
	}
	if !decodeRouteBody(w, r, &input) {
		return
	}
	if !shared.ValidID(input.SourceID) || !shared.ValidID(input.DestinationID) || input.SourceID == input.DestinationID || !shared.ValidProjectFile(input.SourcePath) || !shared.ValidProjectFile(input.DestinationPath) {
		shared.Error(w, 400, "INVALID_PROJECT_FILE")
		return
	}
	unlock := s.operations.Lock(input.SourceID, input.DestinationID)
	defer unlock()
	// Authorize both ends before observing or reading either workspace.
	var project string
	for _, id := range []string{input.SourceID, input.DestinationID} {
		ws, err := s.Store.Workspace(r.Context(), id)
		if err != nil {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if !s.AuthorizeProject(w, r, ws.ProjectID) {
			return
		}
		if project != "" && project != ws.ProjectID {
			shared.Error(w, 403, "CROSS_PROJECT_TRANSFER_DENIED")
			return
		}
		project = ws.ProjectID
	}
	for _, id := range []string{input.SourceID, input.DestinationID} {
		o, err := s.Runtime.Inspect(r.Context(), id)
		if err != nil || !o.Running || !o.Ready {
			shared.Error(w, 409, "WORKSPACES_NOT_READY")
			return
		}
		if !o.Activity.Known || o.Activity.ActiveRuns > 0 || o.Activity.BackgroundJobs > 0 {
			shared.Error(w, 409, "WORKSPACE_BUSY")
			return
		}
	}
	body, _ := json.Marshal(shared.FileHandoff{Path: input.SourcePath})
	status, data, err := s.Runtime.Agent(r.Context(), input.SourceID, "POST", "/v1/files/read", body)
	var file shared.FileHandoff
	if err != nil {
		shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
		return
	}
	if status != 200 || json.Unmarshal(data, &file) != nil || len(file.Content) > shared.HandoffLimit {
		shared.Error(w, 400, "FILE_UNAVAILABLE")
		return
	}
	file.Path = input.DestinationPath
	body, _ = json.Marshal(file)
	status, data, err = s.Runtime.Agent(r.Context(), input.DestinationID, "POST", "/v1/files/write-new", body)
	if err != nil {
		shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
		return
	}
	if status != 201 {
		var failure shared.APIError
		if json.Unmarshal(data, &failure) == nil && failure.Code == "DESTINATION_EXISTS" {
			shared.Error(w, 409, "DESTINATION_EXISTS")
		} else {
			shared.Error(w, 409, "FILE_HANDOFF_FAILED")
		}
		return
	}
	// Content and paths are forwarded in memory, never persisted as platform metadata.
	shared.JSON(w, 201, map[string]any{"copied": true, "bytes": len(file.Content)})
}
