package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"agent-workbench/internal/shared"
)

type principalKey struct{}
type principal struct {
	admin     bool
	projectID string
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == r.Header.Get("Authorization") || got == "" {
			shared.Error(w, 401, "UNAUTHORIZED")
			return
		}
		p := principal{}
		valid := s.cfg.Token != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Token)) == 1
		if valid {
			p.admin = true
		} else if project, err := shared.VerifyProjectAccess(s.cfg.Token, got, s.now()); err == nil {
			p.projectID = project
			valid = true
		} else {
			for token, id := range s.cfg.ProjectTokens {
				if token != "" && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1 && shared.ValidID(id) {
					p.projectID = id
					valid = true
				}
			}
		}
		if !valid {
			shared.Error(w, 401, "UNAUTHORIZED")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

// ProjectForRequest resolves a list filter against verified transport identity.
// Empty project means all memberships only for the local human operator.
func (s *Server) ProjectForRequest(w http.ResponseWriter, r *http.Request, requested string) (string, bool) {
	p, ok := r.Context().Value(principalKey{}).(principal)
	if !ok {
		shared.Error(w, 401, "UNAUTHORIZED")
		return "", false
	}
	if requested == "" {
		requested = p.projectID
	}
	if requested == "" && p.admin {
		return "", true
	}
	if !shared.ValidID(requested) || (!p.admin && p.projectID != requested) {
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return "", false
	}
	if _, err := s.Store.Project(r.Context(), requested); err != nil {
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return "", false
	}
	return requested, true
}
func (s *Server) AuthorizeProject(w http.ResponseWriter, r *http.Request, id string) bool {
	if id == "" {
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return false
	}
	_, ok := s.ProjectForRequest(w, r, id)
	return ok
}
func (s *Server) admin(w http.ResponseWriter, r *http.Request) bool {
	p, ok := r.Context().Value(principalKey{}).(principal)
	if !ok || !p.admin {
		shared.Error(w, 403, "FORBIDDEN")
		return false
	}
	return true
}
func (s *Server) registerDomains(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/projects/{id}/agent-access", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) || !s.AuthorizeProject(w, r, r.PathValue("id")) {
			return
		}
		var input struct {
			TTL int64 `json:"ttl_seconds"`
		}
		if !shared.Decode(w, r, &input, 1024) {
			return
		}
		if input.TTL == 0 {
			input.TTL = 86400
		}
		if input.TTL < 60 || input.TTL > 604800 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		expires := s.now().Add(time.Duration(input.TTL) * time.Second)
		token, err := shared.IssueProjectAccess(s.cfg.Token, r.PathValue("id"), expires)
		if err != nil {
			shared.Error(w, 503, "CREDENTIAL_UNAVAILABLE")
			return
		}
		shared.JSON(w, 201, map[string]any{"project_id": r.PathValue("id"), "access_token": token, "expires_at": expires})
	})
	mux.HandleFunc("GET /v1/projects", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, "")
		if !ok {
			return
		}
		items, err := s.Store.Projects(r.Context())
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		if project != "" {
			filtered := []shared.Project{}
			for _, p := range items {
				if p.ID == project {
					filtered = append(filtered, p)
				}
			}
			items = filtered
		}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /v1/projects", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		var input struct {
			Name string `json:"name"`
			Key  string `json:"idempotency_key"`
		}
		if !shared.Decode(w, r, &input, 1024) {
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || len(input.Name) > 80 || strings.ContainsAny(input.Name, "\r\n\x00") || !shared.ValidKey(input.Key) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		s.createMu.Lock()
		defer s.createMu.Unlock()
		old, err := s.Store.ProjectByKey(r.Context(), input.Key)
		if err == nil {
			if old.Name != input.Name {
				shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
				return
			}
			shared.JSON(w, 200, old)
			return
		}
		if err != sql.ErrNoRows {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		projects, err := s.Store.Projects(r.Context())
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		if len(projects) >= 100 {
			shared.Error(w, 429, "QUOTA_EXCEEDED")
			return
		}
		project := shared.Project{ID: shared.NewID("prj_"), Name: input.Name, CreatedAt: s.now()}
		if s.Store.CreateProject(r.Context(), project, input.Key) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 201, project)
	})
	mux.HandleFunc("GET /v1/agent-homes", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		homes, err := s.Store.AgentHomes(r.Context(), project)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": homes})
	})
	mux.HandleFunc("GET /v1/templates", func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]any{{"id": "development-v1", "name": "Development workspace", "description": "Pinned native T3 and Codex tools in private persistent workspace storage.", "agent_kinds": []string{"t3", "codex"}, "mcps": []string{"workspace-files", "project-git"}, "isolation_status": "unverified"}}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
}
