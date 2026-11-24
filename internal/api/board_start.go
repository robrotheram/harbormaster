package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

// Start a planned outcome in place: retain its task ID, ownership and dependencies.
// The first run has a stable receipt key so reconnects cannot create extra jobs.
func (s *Server) registerBoardStart(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/board/{id}/start", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID   string             `json:"project_id"`
			Revision    int                `json:"revision"`
			WorkspaceID string             `json:"workspace_id,omitempty"`
			Selection   *shared.AgentModel `json:"selection,omitempty"`
		}
		if !decodeRouteBody(w, r, &in) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		id := r.PathValue("id")
		unlock := s.operations.Lock("board:"+id, "work:"+id)
		defer unlock()
		repo := orchestration.Repository{DB: s.Store.DB}
		item, err := repo.Work(r.Context(), in.ProjectID, id)
		if err != nil {
			workError(w, err)
			return
		}
		b, err := s.Store.BoardItem(r.Context(), in.ProjectID, id)
		if err != nil {
			workError(w, err)
			return
		}
		if b.Revision != in.Revision || b.Stage == "archived" {
			shared.Error(w, 409, "BOARD_REVISION_CONFLICT")
			return
		}
		p, _ := r.Context().Value(principalKey{}).(principal)
		if !p.admin && ((p.human.ID != "" && item.ControllerID != "" && item.ControllerID != p.human.ID) || (p.human.ID == "" && item.ControllerID != "")) {
			shared.Error(w, 403, "AGENT_CONTROLLER_REQUIRED")
			return
		}
		key := "board-start-" + id
		if item.LatestAttemptID != "" {
			attempts, err := repo.Attempts(r.Context(), in.ProjectID, id)
			if err != nil {
				workError(w, err)
				return
			}
			// Only replay this action's receipt, never trigger another run.
			var receipt string
			if err = s.Store.DB.QueryRowContext(r.Context(), `SELECT metadata FROM work_attempts WHERE project_id=$1 AND work_item_id=$2 AND delivery_key=$3`, in.ProjectID, id, key).Scan(&receipt); err != nil || len(attempts) == 0 {
				shared.Error(w, 409, "WORK_STATE_CONFLICT")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(receipt))
			return
		}
		if item.State != "planned" && item.State != "draft" && item.State != "in_progress" {
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		if !validExplicitSelection(in.Selection) || (in.WorkspaceID != "" && (!shared.ValidID(in.WorkspaceID) || (item.WorkspaceID != "" && item.WorkspaceID != in.WorkspaceID))) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if !s.checkWorkDependencies(w, r, repo, b) {
			return
		}
		old, _ := json.Marshal(item)
		if in.Selection != nil {
			item.Provider, item.Model, item.ReasoningEffort = in.Selection.Provider, in.Selection.Model, in.Selection.ReasoningEffort
		}
		if s.cfg.RequireAgentSelection && item.Provider == "" {
			workError(w, agentSelectionRequired)
			return
		}
		if in.WorkspaceID != "" {
			item.WorkspaceID = in.WorkspaceID
		}
		if item.WorkspaceID == "" {
			shared.Error(w, http.StatusConflict, "WORKER_SELECTION_REQUIRED")
			return
		}
		ws, err := s.Store.Workspace(r.Context(), item.WorkspaceID)
		if err != nil || !s.workspaceInProject(ws, in.ProjectID) || (in.WorkspaceID != "" && ws.AgentKind == "hermes") {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		release := s.operations.Lock(ws.ID)
		defer release()
		if ws, err = s.Store.Workspace(r.Context(), item.WorkspaceID); err != nil {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if err = s.prepareWorkSubscription(r, item.WorkspaceID, item.Provider); err != nil {
			workError(w, err)
			return
		}
		publish := b.Description != nil || item.SpecificationPath == ""
		if publish || in.Selection != nil {
			if err = s.start(r.Context(), &ws); err != nil {
				apiError(w, err)
				return
			}
		}
		if in.Selection != nil {
			if err = s.checkAgentSelection(r.Context(), ws.ID, in.Selection); err != nil {
				apiError(w, err)
				return
			}
		}
		if publish {
			content := "# " + b.Title + "\n\n"
			if b.Source != nil {
				content += fmt.Sprintf("Source: %s\nRepository: %s\n\n", b.Source.URL, b.Source.Repository)
			}
			if b.Description != nil {
				content += *b.Description
			}
			if len(content) > 65536 {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			digest := sha256.Sum256([]byte(content))
			item.SpecificationDigest = hex.EncodeToString(digest[:])
			item.SpecificationPath = fmt.Sprintf("work-requests/%s-%d.md", id, b.Revision)
			body, _ := json.Marshal(shared.FileHandoff{Path: item.SpecificationPath, Content: []byte(content)})
			status, _, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/files/write-new", body)
			if err != nil {
				shared.Error(w, 503, "PRIVATE_PUBLICATION_FAILED")
				return
			}
			if status == 409 {
				body, _ = json.Marshal(shared.FileHandoff{Path: item.SpecificationPath})
				status, data, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/files/read", body)
				var prior shared.FileHandoff
				if err != nil || status != 200 || json.Unmarshal(data, &prior) != nil || string(prior.Content) != content {
					shared.Error(w, 409, "PRIVATE_PUBLICATION_UNCONFIRMED")
					return
				}
			} else if status != 201 {
				shared.Error(w, 503, "PRIVATE_PUBLICATION_FAILED")
				return
			}
		}
		if item.ControllerID == "" {
			item.ControllerID = p.human.ID
		}
		item.UpdatedAt = s.now()
		data, _ := json.Marshal(item)
		result, err := s.Store.DB.ExecContext(r.Context(), `UPDATE work_items SET metadata=$1 WHERE id=$2 AND project_id=$3 AND metadata=$4`, string(data), id, in.ProjectID, string(old))
		if err != nil {
			workError(w, err)
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		attempt, err := repo.RunWork(r.Context(), in.ProjectID, id, key, s.now())
		if err != nil {
			workError(w, err)
			return
		}
		s.notifyWorkReady()
		shared.JSON(w, 202, attempt)
	})
}

func (s *Server) checkWorkDependencies(w http.ResponseWriter, r *http.Request, repo orchestration.Repository, card store.BoardItem) bool {
	if !card.RequireDependencies {
		return true
	}
	for _, id := range card.Dependencies {
		item, err := repo.Work(r.Context(), card.ProjectID, id)
		if err != nil {
			workError(w, err)
			return false
		}
		dependency, err := s.Store.BoardItem(r.Context(), card.ProjectID, id)
		if err != nil {
			workError(w, err)
			return false
		}
		if dependency.Effective(item).RemoteState() != "closed" {
			shared.Error(w, http.StatusConflict, "WORK_DEPENDENCIES_PENDING")
			return false
		}
	}
	return true
}
