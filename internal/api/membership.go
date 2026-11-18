package api

import (
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) projectAdmin(w http.ResponseWriter, r *http.Request, project string) bool {
	p, _ := r.Context().Value(principalKey{}).(principal)
	if p.admin {
		return s.AuthorizeProject(w, r, project)
	}
	if p.human.ID != "" {
		role, err := s.Store.MemberRole(r.Context(), project, p.human.ID)
		if err == nil && role == "admin" {
			return s.AuthorizeProject(w, r, project)
		}
	}
	shared.Error(w, 403, "PROJECT_ADMIN_REQUIRED")
	return false
}
func membershipError(w http.ResponseWriter, err error) {
	var response error
	if errors.Is(err, store.ErrMembershipConflict) {
		response = shared.CodedError{Status: http.StatusConflict, Code: "MEMBERSHIP_CONFLICT"}
	}
	shared.WriteHTTPError(w, response, shared.CodedError{Status: http.StatusNotFound, Code: "NOT_FOUND_OR_FORBIDDEN"})
}
func (s *Server) ensureCreator(w http.ResponseWriter, r *http.Request, project string) bool {
	p, _ := r.Context().Value(principalKey{}).(principal)
	if p.human.ID == "" {
		return true
	}
	members, err := s.Store.ProjectMembers(r.Context(), project)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return false
	}
	// Never resurrect a revoked creator by replaying project creation.
	if len(members) > 0 {
		return s.AuthorizeProject(w, r, project)
	}
	err = s.Store.SaveMember(r.Context(), store.ProjectMember{ProjectID: project, Email: p.human.Email, UserID: p.human.ID, Role: "admin", Status: "active"}, 0, "", true)
	if err != nil {
		membershipError(w, err)
		return false
	}
	return true
}
func (s *Server) registerMembership(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/project-invitations", func(w http.ResponseWriter, r *http.Request) {
		p, _ := r.Context().Value(principalKey{}).(principal)
		if p.human.ID == "" {
			shared.JSON(w, 200, map[string]any{"items": []any{}})
			return
		}
		items, err := s.Store.Invitations(r.Context(), p.human.Email)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /v1/projects/{id}/accept-invitation", func(w http.ResponseWriter, r *http.Request) {
		p, _ := r.Context().Value(principalKey{}).(principal)
		if p.human.ID == "" {
			shared.Error(w, 403, "HUMAN_IDENTITY_REQUIRED")
			return
		}
		var in struct {
			Revision int `json:"revision"`
		}
		if !shared.Decode(w, r, &in, 1024) {
			return
		}
		err := s.Store.AcceptInvitation(r.Context(), r.PathValue("id"), p.human.Email, p.human.ID, in.Revision)
		if err != nil {
			membershipError(w, err)
			return
		}
		shared.JSON(w, 200, map[string]bool{"accepted": true})
	})
	mux.HandleFunc("GET /v1/projects/{id}/members", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("id")
		if !s.AuthorizeProject(w, r, project) {
			return
		}
		items, err := s.Store.ProjectMembers(r.Context(), project)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		p, _ := r.Context().Value(principalKey{}).(principal)
		role, _ := s.Store.MemberRole(r.Context(), project, p.human.ID)
		shared.JSON(w, 200, map[string]any{"items": items, "can_manage": p.admin || role == "admin", "user_id": p.human.ID})
	})
	mux.HandleFunc("POST /v1/projects/{id}/members", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("id")
		if !s.projectAdmin(w, r, project) {
			return
		}
		var in struct {
			Email    string `json:"email"`
			Role     string `json:"role"`
			Revision int    `json:"revision"`
			Action   string `json:"action"`
		}
		if !shared.Decode(w, r, &in, 2048) {
			return
		}
		in.Email = strings.ToLower(strings.TrimSpace(in.Email))
		if !shared.ValidHuman(shared.HumanIdentity{ID: "user_validation", Email: in.Email}) || (in.Role != "admin" && in.Role != "member") || (in.Action != "invite" && in.Action != "revoke") || in.Revision < 0 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		members, err := s.Store.ProjectMembers(r.Context(), project)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		m := store.ProjectMember{ProjectID: project, Email: in.Email, Role: in.Role, Status: "invited"}
		for _, old := range members {
			if old.Email == in.Email && old.Status == "active" {
				m.UserID = old.UserID
				m.Status = "active"
			}
		}
		if in.Action == "revoke" {
			m.Status = "revoked"
		}
		p, _ := r.Context().Value(principalKey{}).(principal)
		if err = s.Store.SaveMember(r.Context(), m, in.Revision, p.human.ID, p.admin); err != nil {
			membershipError(w, err)
			return
		}
		m.Revision = in.Revision + 1
		shared.JSON(w, 200, m)
	})
}
