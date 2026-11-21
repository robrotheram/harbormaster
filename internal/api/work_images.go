package api

import (
	"bytes"
	"encoding/json"
	"image"
	"net/http"
	"path"
	"regexp"
	"strings"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

var reviewImagePath = regexp.MustCompile(`^work-requests/followup-[a-f0-9]{64}-screenshot-[1-4]\.(png|jpeg)$`)

// Images and chart data referenced by the latest reply may not be retained
// artifacts. Read them from that work's private workspace, never a host path.
func (s *Server) registerWorkImages(mux *http.ServeMux, repo orchestration.Repository) {
	for _, kind := range []string{"image", "data"} {
		mux.HandleFunc("POST /v1/work-items/{id}/"+kind, func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				ProjectID string `json:"project_id"`
				AttemptID string `json:"attempt_id"`
				Path      string `json:"path"`
			}
			if !decodeRouteBody(w, r, &in) || !s.AuthorizeProject(w, r, in.ProjectID) {
				return
			}
			if !shared.ValidProjectFile(in.Path) || !shared.ValidID(in.AttemptID) {
				shared.Error(w, 400, "INVALID_PROJECT_FILE")
				return
			}
			ext := strings.ToLower(path.Ext(in.Path))
			allowed := false
			limit := shared.HandoffLimit
			if kind == "data" {
				allowed = ext == ".csv" || ext == ".json"
				limit = 256 << 10
			} else {
				switch ext {
				case ".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif":
					allowed = true
				}
			}
			if !allowed {
				shared.Error(w, 400, "INVALID_PROJECT_FILE")
				return
			}
			item, err := repo.Work(r.Context(), in.ProjectID, r.PathValue("id"))
			if err != nil {
				workError(w, err)
				return
			}
			workspace := item.WorkspaceID
			historical := item.LatestAttemptID != in.AttemptID
			if historical {
				// An earlier reply can only read the write-once review images
				// named for its own follow-up specification.
				if kind != "image" || !reviewImagePath.MatchString(in.Path) {
					shared.Error(w, 404, "OUTPUT_NOT_RETAINED")
					return
				}
				attempt, err := repo.Attempt(r.Context(), in.ProjectID, item.ID, in.AttemptID)
				specification, isFollowup := strings.CutSuffix(attempt.SpecificationPath, ".md")
				if err != nil || !isFollowup || !strings.HasPrefix(in.Path, specification+"-screenshot-") {
					shared.Error(w, 404, "OUTPUT_NOT_RETAINED")
					return
				}
				workspace = attempt.WorkspaceID
			}
			ws, err := s.Store.Workspace(r.Context(), workspace)
			if err != nil || !s.workspaceInProject(ws, in.ProjectID) {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
			unlock := s.operations.Lock(ws.ID)
			defer unlock()
			if err := s.lifecycle().ObserveTask(r.Context(), &ws); err != nil {
				shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
				return
			}
			body, _ := json.Marshal(shared.FileHandoff{Path: in.Path})
			status, data, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/files/read", body)
			if err != nil {
				shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
				return
			}
			if status != 200 {
				shared.Error(w, status, "FILE_UNAVAILABLE")
				return
			}
			var file shared.FileHandoff
			if json.Unmarshal(data, &file) != nil || file.Path != in.Path || len(file.Content) > limit {
				shared.Error(w, 503, "FILE_UNAVAILABLE")
				return
			}
			if historical {
				_, format, err := image.DecodeConfig(bytes.NewReader(file.Content))
				if err != nil || "."+format != ext {
					shared.Error(w, 503, "FILE_UNAVAILABLE")
					return
				}
			}
			shared.JSON(w, 200, file)
		})
	}
}
