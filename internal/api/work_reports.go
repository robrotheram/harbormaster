package api

import (
	"encoding/json"
	"net/http"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

func (s *Server) registerWorkReports(mux *http.ServeMux, repo orchestration.Repository) {
	mux.HandleFunc("POST /v1/work-items/{id}/reports", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID string `json:"project_id"`
			WorkerID  string `json:"worker_id"`
			shared.ReportWrite
		}
		if !decodeRouteBody(w, r, &in) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		id := r.PathValue("id")
		unlockWork := s.operations.Lock("work:" + id)
		defer unlockWork()
		if !s.authorizeWorkController(w, r, in.ProjectID, id, repo) {
			return
		}
		if !in.Valid() {
			shared.Error(w, 400, "INVALID_REPORT")
			return
		}
		ctx := r.Context()
		item, err := repo.Work(ctx, in.ProjectID, id)
		if err != nil {
			workError(w, err)
			return
		}
		if item.Kind != shared.WorkKindDataScience || item.WorkspaceID != in.WorkerID {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		ws, err := s.Store.Workspace(ctx, item.WorkspaceID)
		if err != nil || !s.workspaceInProject(ws, in.ProjectID) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		unlockWorker := s.operations.Lock(ws.ID)
		defer unlockWorker()
		if err := s.lifecycle().ObserveTask(ctx, &ws); err != nil {
			shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
			return
		}
		body, _ := json.Marshal(in.ReportWrite)
		status, reply, err := s.Runtime.Agent(ctx, ws.ID, "POST", "/v1/files/write-report", body)
		if err == nil && status == 200 {
			shared.JSON(w, 200, map[string]string{"path": in.Path})
			return
		}
		var failure shared.APIError
		if err == nil && status == 409 && json.Unmarshal(reply, &failure) == nil {
			switch failure.Code {
			case "REPORT_CHANGED", "WORKSPACE_BUSY":
				shared.Error(w, 409, failure.Code)
				return
			}
		}
		shared.Error(w, 503, "REPORT_SAVE_FAILED")
	})
}
