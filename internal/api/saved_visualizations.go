package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
)

func (s *Server) visualizationDetails(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("id")
	if !s.AuthorizeProject(w, r, project) {
		return
	}
	p, _ := r.Context().Value(principalKey{}).(principal)
	owner := visualizationOwner(p)
	if owner == "" {
		shared.Error(w, 403, "FORBIDDEN")
		return
	}
	if r.Method == http.MethodGet {
		edits, err := s.Store.VisualizationEdits(r.Context(), project, owner)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"titles": edits})
		return
	}
	var item store.VisualizationDetails
	if !decodeRouteBody(w, r, &item) {
		return
	}
	if !shared.ValidID(item.WorkerID) || !shared.ValidID(item.ArtifactID) ||
		(item.Title == nil && item.Description == nil) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	s.editVisualizationDetails(w, r, p, project, owner, item)
}

func visualizationOwner(p principal) string {
	if p.admin {
		return "local-operator"
	}
	if p.human.ID != "" {
		return "user:" + p.human.ID
	}
	return ""
}

func (s *Server) savedVisualizations(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("id")
	if !s.AuthorizeProject(w, r, project) {
		return
	}
	p, _ := r.Context().Value(principalKey{}).(principal)
	owner := visualizationOwner(p)
	if owner == "" {
		shared.Error(w, 403, "FORBIDDEN")
		return
	}

	ctx := r.Context()
	if r.Method == http.MethodGet {
		items, err := s.Store.SavedVisualizations(ctx, project, owner)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		edits, err := s.Store.VisualizationEdits(ctx, project, owner)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": items, "titles": edits})
		return
	}

	var input struct {
		store.VisualizationDetails
		Saved *bool `json:"saved"`
	}
	if !decodeRouteBody(w, r, &input) {
		return
	}
	editDetails := input.Title != nil || input.Description != nil
	if !shared.ValidID(input.WorkerID) || !shared.ValidID(input.ArtifactID) ||
		(input.Saved != nil) == editDetails {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	if editDetails {
		s.editVisualizationDetails(w, r, p, project, owner, input.VisualizationDetails)
		return
	}
	if !*input.Saved {
		// A saved reference can be removed after its worker has been deleted.
		if err := s.Store.RemoveSavedVisualization(ctx, project, owner, input.WorkerID, input.ArtifactID); err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]bool{"saved": false})
		return
	}

	item, ok := s.loadWorkerVisualization(w, r, p, project, input.WorkerID, input.ArtifactID)
	if !ok {
		return
	}
	if err := s.Store.SaveVisualization(ctx, project, owner, item, s.now().UnixNano()); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, map[string]bool{"saved": true})
}

func (s *Server) editVisualizationDetails(w http.ResponseWriter, r *http.Request, p principal, project, owner string, item store.VisualizationDetails) {
	if item.Title != nil {
		title := strings.TrimSpace(*item.Title)
		if title == "" || utf8.RuneCountInString(title) > 200 || strings.IndexFunc(title, unicode.IsControl) >= 0 {
			shared.Error(w, 400, "INVALID_VISUALIZATION_TITLE")
			return
		}
		item.Title = &title
	}
	if item.Description != nil {
		description := strings.TrimSpace(*item.Description)
		if utf8.RuneCountInString(description) > 2000 || strings.IndexFunc(description, func(r rune) bool {
			return unicode.IsControl(r) && r != '\n' && r != '\t'
		}) >= 0 {
			shared.Error(w, 400, "INVALID_VISUALIZATION_DESCRIPTION")
			return
		}
		item.Description = &description
	}
	ctx := r.Context()
	known, err := s.Store.HasVisualizationReference(ctx, project, owner, item.WorkerID, item.ArtifactID)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	if !known {
		if _, ok := s.loadWorkerVisualization(w, r, p, project, item.WorkerID, item.ArtifactID); !ok {
			return
		}
	}
	if err := s.Store.UpdateVisualizationDetails(ctx, project, owner, item); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, item)
}

func (s *Server) loadWorkerVisualization(w http.ResponseWriter, r *http.Request, p principal, project, worker, artifact string) (store.SavedVisualization, bool) {
	ctx := r.Context()
	ws, err := s.Store.Workspace(ctx, worker)
	if err != nil || !s.workspaceInProject(ws, project) || ws.ProjectID != project || ws.Shared && !p.admin {
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return store.SavedVisualization{}, false
	}
	status, data, err := s.Runtime.Agent(ctx, ws.ID, http.MethodGet, "/v1/analysis/artifacts/"+artifact, nil)
	if err == nil && status == http.StatusNotFound {
		shared.Error(w, 404, "ANALYSIS_ARTIFACT_UNAVAILABLE")
		return store.SavedVisualization{}, false
	}
	var spec struct {
		store.SavedVisualization
		SchemaVersion string `json:"schema_version"`
	}
	if err != nil || status != http.StatusOK || len(data) > shared.AnalysisResponseLimit ||
		json.Unmarshal(data, &spec) != nil {
		shared.Error(w, 503, "RUNTIME_UNAVAILABLE")
		return store.SavedVisualization{}, false
	}
	if spec.SchemaVersion != "1.0" || spec.ArtifactID != artifact || spec.Title == "" ||
		(spec.Type != "table" && spec.Type != "chart" && spec.Type != "geo") {
		shared.Error(w, 422, "INVALID_ANALYSIS_ARTIFACT")
		return store.SavedVisualization{}, false
	}
	spec.WorkerID = ws.ID
	return spec.SavedVisualization, true
}
