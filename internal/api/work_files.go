package api

import (
	"encoding/json"
	"net/http"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

func (s *Server) registerWorkFiles(mux *http.ServeMux, repo orchestration.Repository) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		mux.HandleFunc(method+" /v1/work-items/{id}/files", func(w http.ResponseWriter, r *http.Request) {
			project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
			if !ok {
				return
			}
			if project == "" {
				shared.Error(w, 400, "PROJECT_REQUIRED")
				return
			}
			if s.cfg.SharedWorkspaceID != "" && !s.admin(w, r) {
				return
			}
			var input struct {
				Path string `json:"path"`
			}
			endpoint := "/v1/files"
			var body []byte
			if method == http.MethodPost {
				if !decodeRouteBody(w, r, &input) {
					return
				}
				if !shared.ValidProjectFile(input.Path) {
					shared.Error(w, 400, "INVALID_PROJECT_FILE")
					return
				}
				endpoint += "/inspect"
				body, _ = json.Marshal(input)
			}
			ctx := r.Context()
			item, err := repo.Work(ctx, project, r.PathValue("id"))
			if err != nil {
				workError(w, err)
				return
			}
			ws, err := s.Store.Workspace(ctx, item.WorkspaceID)
			if err != nil || !s.workspaceInProject(ws, project) {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
			unlock := s.operations.Lock(ws.ID)
			defer unlock()
			if err := s.lifecycle().ObserveTask(ctx, &ws); err != nil {
				shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
				return
			}
			status, data, err := s.Runtime.Agent(ctx, ws.ID, method, endpoint, body)
			if method == http.MethodGet {
				var files shared.WorkspaceFiles
				if err != nil || status != 200 || json.Unmarshal(data, &files) != nil || files.Paths == nil {
					shared.Error(w, 503, "WORKSPACE_FILES_UNAVAILABLE")
					return
				}
				shared.JSON(w, 200, files)
				return
			}
			var file shared.FileHandoff
			switch {
			case err != nil:
				shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
			case status == 413:
				shared.Error(w, status, "WORKSPACE_FILE_TOO_LARGE")
			case status == 400 || status == 404:
				shared.Error(w, status, "FILE_UNAVAILABLE")
			case status != 200 || json.Unmarshal(data, &file) != nil || file.Path != input.Path || len(file.Content) > shared.HandoffLimit:
				shared.Error(w, 503, "FILE_UNAVAILABLE")
			default:
				shared.JSON(w, 200, file)
			}
		})
	}
}
