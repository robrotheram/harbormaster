package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

type principalKey struct{}
type principal struct {
	human     shared.HumanIdentity
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
		} else if human, err := shared.VerifyHumanAccess(s.cfg.Token, got, s.now()); err == nil {
			p.human = human
			valid = true
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
		// Shared-worker mode intentionally crosses project storage boundaries. It is
		// not an execution environment for explicit per-project human membership.
		if p.human.ID != "" && s.cfg.SharedWorkspaceID != "" {
			path := r.URL.Path
			parts := strings.Split(strings.Trim(path, "/"), "/")
			projectMetadata := path == "/v1/projects" || (len(parts) == 4 && parts[1] == "projects" && (parts[3] == "members" || parts[3] == "accept-invitation" || parts[3] == "rename" || parts[3] == "delete" || parts[3] == "capabilities"))
			boardMetadata := (path == "/v1/board" || (len(parts) == 3 && parts[1] == "board")) && r.Method == "POST"
			metadata := boardMetadata || projectMetadata || path == "/v1/project-invitations" || path == "/v1/templates" || (r.Method == "GET" && (path == "/v1/board" || path == "/v1/work-items" || (strings.HasPrefix(path, "/v1/work-items/") && (len(parts) == 3 || strings.HasSuffix(path, "/attempts")))))
			if !metadata {
				shared.Error(w, 409, "PROJECT_ISOLATED_WORKERS_REQUIRED")
				return
			}
		}
		ctx := context.WithValue(r.Context(), principalKey{}, p)
		if !p.admin {
			actor := p.human.ID
			if actor == "" {
				actor = "project-credential"
			}
			ctx = orchestration.WithWorkActor(ctx, actor)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
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
	if p.human.ID != "" {
		if !shared.ValidID(requested) {
			shared.Error(w, 400, "PROJECT_REQUIRED")
			return "", false
		}
		if _, err := s.Store.MemberRole(r.Context(), requested, p.human.ID); err != nil {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return "", false
		}
	}
	if !shared.ValidID(requested) || (!p.admin && p.human.ID == "" && p.projectID != requested) {
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
	s.registerMembership(mux)
	mux.HandleFunc("GET /v1/projects/{id}/capabilities", func(w http.ResponseWriter, r *http.Request) {
		if !s.AuthorizeProject(w, r, r.PathValue("id")) {
			return
		}
		p, _ := r.Context().Value(principalKey{}).(principal)
		shared.JSON(w, 200, map[string]any{"can_import": p.admin, "can_start": p.admin || s.cfg.SharedWorkspaceID == "", "import_reason": "Repository imports use platform-managed connections. Ask a platform operator to import issues into this project.", "start_reason": "Starting agents requires a project-isolated worker. Ask a platform operator to configure project isolation."})
	})
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
		p, _ := r.Context().Value(principalKey{}).(principal)
		project, ok := "", true
		if p.human.ID == "" {
			project, ok = s.ProjectForRequest(w, r, "")
		}
		if !ok {
			return
		}
		items, err := s.Store.Projects(r.Context())
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		if project != "" || p.human.ID != "" {
			filtered := []shared.Project{}
			for _, item := range items {
				role, _ := s.Store.MemberRole(r.Context(), item.ID, p.human.ID)
				if item.ID == project || (p.human.ID != "" && role != "") {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /v1/projects", func(w http.ResponseWriter, r *http.Request) {
		p, _ := r.Context().Value(principalKey{}).(principal)
		if p.human.ID == "" && !s.admin(w, r) {
			return
		}
		var input struct {
			Name   string `json:"name"`
			Key    string `json:"idempotency_key"`
			Sample bool   `json:"sample,omitempty"`
		}
		if !shared.Decode(w, r, &input, 1024) {
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || len(input.Name) > 80 || strings.ContainsAny(input.Name, "\r\n\x00") || !shared.ValidKey(input.Key) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if p.human.ID != "" {
			input.Key = p.human.ID + ":" + input.Key
		}
		s.createMu.Lock()
		defer s.createMu.Unlock()
		old, err := s.Store.ProjectByKey(r.Context(), input.Key)
		if err == nil {
			if old.Name != input.Name || old.Sample != input.Sample {
				shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
				return
			}
			if !s.provisionProjectNamespace(w, r, &old) {
				return
			}
			if !s.ensureCreator(w, r, old.ID) {
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
		project := shared.Project{ID: shared.NewID("prj_"), Name: input.Name, Sample: input.Sample, CreatedAt: s.now()}
		if s.cfg.ProvisionProjectNamespaces {
			project.DeploymentNamespace = shared.ProjectNamespace(project.ID)
		}
		if s.Store.CreateProject(r.Context(), project, input.Key) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		if !s.provisionProjectNamespace(w, r, &project) {
			return
		}
		if !s.ensureCreator(w, r, project.ID) {
			return
		}
		shared.JSON(w, 201, project)
	})
	mux.HandleFunc("POST /v1/projects/{id}/rename", func(w http.ResponseWriter, r *http.Request) {
		if !s.projectAdmin(w, r, r.PathValue("id")) {
			return
		}
		var input struct {
			Name string `json:"name"`
		}
		if !shared.Decode(w, r, &input, 1024) {
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || len(input.Name) > 80 || strings.ContainsAny(input.Name, "\r\n\x00") {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		s.createMu.Lock()
		defer s.createMu.Unlock()
		project, err := s.Store.RenameProject(r.Context(), r.PathValue("id"), input.Name)
		if err == sql.ErrNoRows {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, project)
	})
	mux.HandleFunc("GET /v1/templates", func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]any{{"id": "development-v1", "name": "Development workspace", "description": "Pinned native T3 and Codex tools in private persistent workspace storage.", "agent_kinds": []string{"t3", "codex"}, "mcps": []string{"workspace-files", "project-git"}, "isolation_status": "unverified"}}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
}
