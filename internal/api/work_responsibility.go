package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"encoding/json"
	"net/http"
)

func (s *Server) authorizeWorkController(w http.ResponseWriter, r *http.Request, project, id string, repo orchestration.Repository) bool {
	item, err := repo.Work(r.Context(), project, id)
	if err != nil {
		workError(w, err)
		return false
	}
	p, _ := r.Context().Value(principalKey{}).(principal)
	if p.admin || (p.human.ID != "" && item.ControllerID == p.human.ID) || (p.human.ID == "" && item.ControllerID == "") {
		return true
	}
	shared.Error(w, 403, "AGENT_CONTROLLER_REQUIRED")
	return false
}
func (s *Server) registerWorkResponsibility(mux *http.ServeMux, repo orchestration.Repository) {
	mux.HandleFunc("POST /v1/work-items/{id}/responsibility", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID          string `json:"project_id"`
			Controller         string `json:"controller_id"`
			Reviewer           string `json:"reviewer_id"`
			ExpectedController string `json:"expected_controller_id"`
			ExpectedReviewer   string `json:"expected_reviewer_id"`
			ExpectedAttempt    string `json:"expected_attempt_id"`
		}
		if !shared.Decode(w, r, &in, 4096) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		unlock := s.operations.Lock("work:" + r.PathValue("id"))
		defer unlock()
		item, err := repo.Work(r.Context(), in.ProjectID, r.PathValue("id"))
		if err != nil {
			workError(w, err)
			return
		}
		p, _ := r.Context().Value(principalKey{}).(principal)
		role, _ := s.Store.MemberRole(r.Context(), in.ProjectID, p.human.ID)
		if !p.admin && (p.human.ID == "" || (role != "admin" && item.ControllerID != "" && item.ControllerID != p.human.ID)) {
			shared.Error(w, 403, "CONTROLLER_OR_ADMIN_REQUIRED")
			return
		}
		if !p.admin && role != "admin" && item.ControllerID == "" && in.Controller != p.human.ID {
			shared.Error(w, 403, "CLAIM_CONTROL_FIRST")
			return
		}
		if item.ControllerID != in.ExpectedController || item.ReviewerID != in.ExpectedReviewer || item.LatestAttemptID != in.ExpectedAttempt {
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		switch item.State {
		case "draft", "planned", "in_progress", "awaiting_review", "completed", "closed", "changes_requested", "failed", "cancelled":
		default:
			shared.Error(w, 409, "STOP_RUN_BEFORE_HANDOFF")
			return
		}
		for _, id := range []string{in.Controller, in.Reviewer} {
			if id != "" {
				if _, err = s.Store.MemberRole(r.Context(), in.ProjectID, id); err != nil {
					shared.Error(w, 400, "ACTIVE_MEMBER_REQUIRED")
					return
				}
			}
		}
		if len(item.ControlHistory) >= 1000 {
			shared.Error(w, 409, "HANDOFF_HISTORY_LIMIT")
			return
		}
		old, _ := json.Marshal(item)
		if item.ControllerID != in.Controller {
			item.ControlHistory = append(item.ControlHistory, shared.ControlHandoff{From: item.ControllerID, To: in.Controller, Actor: p.human.ID, At: s.now()})
		}
		item.ControllerID = in.Controller
		item.ReviewerID = in.Reviewer
		item.UpdatedAt = s.now()
		data, _ := json.Marshal(item)
		result, err := s.Store.DB.ExecContext(r.Context(), `UPDATE work_items SET metadata=$1 WHERE project_id=$2 AND id=$3 AND metadata=$4`, string(data), in.ProjectID, item.ID, string(old))
		if err != nil {
			workError(w, err)
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		shared.JSON(w, 200, item)
	})
}
