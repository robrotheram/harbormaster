package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
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
			ProjectID string `json:"project_id"`
			Revision  int    `json:"revision"`
		}
		if !shared.Decode(w, r, &in, 2048) || !s.AuthorizeProject(w, r, in.ProjectID) {
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
		old, _ := json.Marshal(item)
		if item.WorkspaceID == "" {
			response := &coordinationResponse{headers: make(http.Header)}
			agentKind := "codex"
			if s.cfg.SharedWorkspaceID != "" {
				agentKind = "t3"
			}
			s.createProjectWorkspace(response, r, workspaceCreate{Project: in.ProjectID, Key: key, AgentKind: agentKind, TemplateID: "development-v1"})
			if response.Code >= 300 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(response.Code)
				_, _ = w.Write(response.Body.Bytes())
				return
			}
			var ws shared.Workspace
			if json.Unmarshal(response.Body.Bytes(), &ws) != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			item.WorkspaceID = ws.ID
		}
		if b.Description != nil || item.SpecificationPath == "" {
			content := "# " + b.Title + "\n\n"
			if b.Description != nil {
				content += *b.Description
			}
			if len(content) > 65536 {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			ws, err := s.Store.Workspace(r.Context(), item.WorkspaceID)
			if err != nil || !s.workspaceInProject(ws, in.ProjectID) {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
			release := s.operations.Lock(ws.ID)
			defer release()
			if err = s.start(r.Context(), &ws); err != nil {
				apiError(w, err)
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
		shared.JSON(w, 202, attempt)
	})
}
