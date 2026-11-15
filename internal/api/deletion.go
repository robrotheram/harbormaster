package api

import (
	"agent-workbench/internal/shared"
	"net/http"
)

func (s *Server) registerDeletion(mux *http.ServeMux) {
	for _, kind := range []string{"workspaces", "projects"} {
		mux.HandleFunc("POST /v1/"+kind+"/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
			if !s.admin(w, r) {
				return
			}
			var in struct {
				Confirm string `json:"confirm_id"`
			}
			if !shared.Decode(w, r, &in, 1024) {
				return
			}
			id := r.PathValue("id")
			if !shared.ValidID(id) || in.Confirm != id {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			s.createMu.Lock()
			defer s.createMu.Unlock()
			var targets []shared.Workspace
			project := ""
			if kind == "projects" {
				if id == "default" {
					shared.Error(w, 409, "LEGACY_PROJECT_DELETE_UNSUPPORTED")
					return
				}
				if !s.AuthorizeProject(w, r, id) {
					return
				}
				project = id
				all, err := s.Store.Workspaces(r.Context())
				if err != nil {
					shared.Error(w, 503, "STORAGE_UNAVAILABLE")
					return
				}
				for _, ws := range all {
					if ws.ProjectID == id {
						targets = append(targets, ws)
					}
				}
			} else {
				ws, ok := s.workspace(w, r)
				if !ok {
					return
				}
				if s.sharedWorkspace(ws) {
					shared.Error(w, 409, "SHARED_WORKSPACE_ONLY")
					return
				}
				targets = append(targets, ws)
			}
			ids := []string{}
			for _, ws := range targets {
				ids = append(ids, ws.ID)
			}
			unlock := s.operations.Lock(ids...)
			defer unlock()
			// Re-read under lifecycle locks. Unknown/busy state or failed stop prevents deletion.
			for i, old := range targets {
				ws, err := s.Store.Workspace(r.Context(), old.ID)
				if err != nil {
					shared.Error(w, 409, "WORKSPACE_CHANGED")
					return
				}
				if err = s.stop(r.Context(), &ws); err != nil {
					apiError(w, err)
					return
				}
				if ws.State != "STOPPED" {
					shared.Error(w, 409, "STOP_WORKSPACE_FIRST")
					return
				}
				targets[i] = ws
			}
			if err := s.Store.DeleteStopped(r.Context(), targets, project); err != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			shared.JSON(w, 200, map[string]any{"id": id, "deleted": true, "private_disks": "retained"})
		})
	}
}
