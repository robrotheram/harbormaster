package api

import (
	"agent-workbench/internal/matrixsetup"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func validHermesSetup(c shared.HermesSetup) bool {
	if c.Provider != "openai-codex" || len(c.Model) == 0 || len(c.Model) > 128 || c.Revision < 0 || c.Revision > 1000000000 {
		return false
	}
	for _, r := range c.Model {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:/-", r)) {
			return false
		}
	}
	if c.ConnectionProfileID != "" && !shared.ValidID(c.ConnectionProfileID) {
		return false
	}
	if c.Mode == "workspace" {
		return c.Matrix == nil
	}
	if c.Mode == "both" && c.Matrix == nil {
		return true
	}
	if c.Mode != "matrix" && c.Mode != "both" || c.Matrix == nil {
		return false
	}
	m := c.Matrix
	u, e := url.Parse(m.Homeserver)
	if e != nil || len(m.Homeserver) > 512 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	validID := func(v, prefix string) bool {
		return len(v) > 1 && len(v) <= 255 && strings.HasPrefix(v, prefix) && !strings.ContainsAny(v, " \t\r\n\x00*?#/")
	}
	if !validID(m.BotUserID, "@") || !strings.Contains(m.BotUserID, ":") || len(m.RoomIDs) == 0 || len(m.RoomIDs) > 16 || len(m.SenderIDs) == 0 || len(m.SenderIDs) > 32 {
		return false
	}
	for _, group := range []struct {
		items  []string
		prefix string
	}{{m.RoomIDs, "!"}, {m.SenderIDs, "@"}} {
		seen := map[string]bool{}
		for _, v := range group.items {
			if !validID(v, group.prefix) || seen[v] || group.prefix == "@" && !strings.Contains(v, ":") {
				return false
			}
			seen[v] = true
		}
	}
	return true
}
func (s *Server) hermesAvailable(ctx context.Context) bool {
	return s.hermesRuntimeStatus(ctx) == "available"
}
func (s *Server) hermesRuntimeStatus(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	kinds, err := s.Runtime.AgentKinds(ctx)
	if err != nil {
		return "unavailable"
	}
	for _, kind := range kinds {
		if kind == "hermes" {
			return "available"
		}
	}
	return "not_installed"
}
func (s *Server) hermesResponse(ctx context.Context, c shared.HermesSetup) map[string]any {
	status := s.hermesRuntimeStatus(ctx)
	available := status == "available"
	result := map[string]any{"config": c, "saved": c.Revision > 0, "runtime_available": available, "runtime_status": status, "matrix_available": available, "matrix_credential_configured": false, "matrix_state": "disconnected"}
	if c.Matrix != nil {
		result["matrix_credential_configured"] = s.Runtime.MatrixCredentialPresent(ctx, c.ProjectID)
	}
	if workspaces, err := s.Store.Workspaces(ctx); err == nil {
		for _, ws := range workspaces {
			if ws.ProjectID == c.ProjectID && ws.AgentKind == "hermes" {
				result["workspace_id"] = ws.ID
				result["workspace_ready"] = ws.Ready
				if ws.Ready {
					code, b, err := s.Runtime.Agent(ctx, ws.ID, "GET", "/v1/matrix/status", nil)
					var state struct {
						State string `json:"state"`
					}
					if err == nil && code == 200 && json.Unmarshal(b, &state) == nil {
						result["matrix_state"] = state.State
					}
				}
				break
			}
		}
	}
	return result
}

type matrixOnboarding interface {
	Discover(context.Context, string) (string, error)
	Prepare(context.Context, shared.HermesMatrix, string, bool) error
}

func matrixSetupError(w http.ResponseWriter, err error) {
	code := err.Error()
	switch code {
	case "MATRIX_HOMESERVER_INVALID", "MATRIX_TOKEN_INVALID", "MATRIX_BOT_ID_MISMATCH", "MATRIX_BOT_OUTSIDE_ROOMS", "MATRIX_INVITE_REQUIRED", "MATRIX_ACCEPT_INVITATIONS_REQUIRED", "MATRIX_ROOM_ENCRYPTION_REQUIRED":
		shared.Error(w, 400, code)
	default:
		shared.Error(w, 503, "MATRIX_UNAVAILABLE")
	}
}
func (s *Server) onboarding() matrixOnboarding {
	if s.matrixSetup != nil {
		return s.matrixSetup
	}
	return matrixsetup.New()
}
func (s *Server) registerHermes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/projects/{id}/hermes/assistant", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !s.AuthorizeProject(w, r, id) {
			return
		}
		var input struct {
			Key string `json:"idempotency_key"`
		}
		if !shared.Decode(w, r, &input, 1024) {
			return
		}
		if !shared.ValidKey(input.Key) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		projectUnlock := s.operations.Lock("project:" + id)
		defer projectUnlock()
		ws, created, err := s.lifecycle().EnsureAssistant(r.Context(), s.Store, id, input.Key, s.cfg.WorkspaceMemory, func() func() { s.createMu.Lock(); return s.createMu.Unlock })
		if err != nil {
			apiError(w, err)
			return
		}
		status := 200
		if created {
			status = 201
		}
		shared.JSON(w, status, ws)
	})

	mux.HandleFunc("POST /v1/projects/{id}/hermes/discover", func(w http.ResponseWriter, r *http.Request) {
		if !s.projectAdmin(w, r, r.PathValue("id")) {
			return
		}
		var body struct {
			Homeserver string `json:"homeserver"`
		}
		if !shared.Decode(w, r, &body, 2048) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		origin, err := s.onboarding().Discover(ctx, body.Homeserver)
		if err != nil {
			matrixSetupError(w, err)
			return
		}
		shared.JSON(w, 200, map[string]string{"homeserver": origin})
	})
	mux.HandleFunc("POST /v1/projects/{id}/hermes/matrix-credential", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !s.projectAdmin(w, r, id) {
			return
		}
		var body struct {
			Token             string `json:"access_token"`
			Revision          int    `json:"revision"`
			AcceptInvitations bool   `json:"accept_invitations"`
		}
		if !shared.Decode(w, r, &body, 12288) {
			return
		}
		c, err := s.Store.HermesSetup(r.Context(), id)
		if err != nil || c.Matrix == nil {
			shared.Error(w, 409, "MATRIX_SETTINGS_REQUIRED")
			return
		}
		// The operator must be saving a credential for the settings they saw.
		// A project agent changing the homeserver must not redirect this token.
		if body.Revision != c.Revision {
			shared.Error(w, 409, "HERMES_REVISION_CONFLICT")
			return
		}
		if !s.hermesAvailable(r.Context()) {
			shared.Error(w, 409, "HERMES_SETUP_REQUIRED")
			return
		}
		body.Token = strings.TrimSpace(body.Token)
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		err = s.onboarding().Prepare(ctx, *c.Matrix, body.Token, body.AcceptInvitations)
		cancel()
		if err != nil {
			matrixSetupError(w, err)
			return
		}
		latest, e := s.Store.HermesSetup(r.Context(), id)
		if e != nil || latest.Revision != body.Revision {
			shared.Error(w, 409, "HERMES_REVISION_CONFLICT")
			return
		}
		if s.Runtime.MatrixCredential(r.Context(), id, *c.Matrix, body.Token) != nil {
			shared.Error(w, 400, "MATRIX_CREDENTIAL_REJECTED")
			return
		}
		shared.JSON(w, 200, map[string]bool{"credential_configured": true})
	})
	for _, action := range []string{"start", "stop"} {
		action := action
		mux.HandleFunc("POST /v1/projects/{id}/hermes/matrix/"+action, func(w http.ResponseWriter, r *http.Request) {
			id := r.PathValue("id")
			if !s.projectAdmin(w, r, id) {
				return
			}
			c, err := s.Store.HermesSetup(r.Context(), id)
			if err != nil || (action == "start" && c.Matrix == nil) {
				shared.Error(w, 409, "MATRIX_SETTINGS_REQUIRED")
				return
			}
			projectUnlock := s.operations.Lock("project:" + id)
			defer projectUnlock()
			items, err := s.Store.Workspaces(r.Context())
			if err != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			var ws shared.Workspace
			for _, item := range items {
				if item.ProjectID == id && item.AgentKind == "hermes" {
					ws = item
					break
				}
			}
			if ws.ID == "" {
				shared.Error(w, 409, "HERMES_WORKSPACE_REQUIRED")
				return
			}
			unlock := s.operations.Lock(ws.ID)
			defer unlock()
			ws, err = s.Store.Workspace(r.Context(), ws.ID)
			if err != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			if err = s.refresh(r.Context(), &ws); err != nil || !ws.Ready {
				shared.Error(w, 409, "NOT_READY")
				return
			}
			if action == "start" {
				err = s.Runtime.MatrixAction(r.Context(), ws.ID, "enable", c.Matrix)
				if err != nil && err.Error() == "MATRIX_RESTART_REQUIRED" {
					if ws.ActiveRuns > 0 || ws.BackgroundJobs > 0 {
						shared.Error(w, 409, "WORKSPACE_BUSY")
						return
					}
					if e := s.stop(r.Context(), &ws); e != nil {
						apiError(w, e)
						return
					}
					if e := s.start(r.Context(), &ws); e != nil {
						apiError(w, e)
						return
					}
					err = s.Runtime.MatrixAction(r.Context(), ws.ID, "enable", c.Matrix)
				}
				if err != nil {
					code := err.Error()
					if code != "MATRIX_RESTART_REQUIRED" && code != "MATRIX_ALREADY_CONNECTED" {
						code = "MATRIX_CREDENTIAL_REJECTED"
					}
					shared.Error(w, 409, code)
					return
				}
			}
			code, b, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/matrix/"+action, nil)
			if err != nil || (code != 200 && code != 202) {
				_ = s.Runtime.MatrixAction(r.Context(), ws.ID, "disable", nil)
				shared.Error(w, 503, "MATRIX_UNAVAILABLE")
				return
			}
			if action == "stop" {
				_ = s.Runtime.MatrixAction(r.Context(), ws.ID, "disable", nil)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			w.Write(b)
		})
	}
	mux.HandleFunc("GET /v1/projects/{id}/hermes", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !s.AuthorizeProject(w, r, id) {
			return
		}
		c, err := s.Store.HermesSetup(r.Context(), id)
		if err == sql.ErrNoRows {
			c = shared.HermesSetup{ProjectID: id, Provider: "openai-codex", Mode: "both"}
		} else if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, s.hermesResponse(r.Context(), c))
	})
	mux.HandleFunc("POST /v1/projects/{id}/hermes", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !s.AuthorizeProject(w, r, id) {
			return
		}
		var c shared.HermesSetup
		if !shared.Decode(w, r, &c, 16384) {
			return
		}
		if c.ProjectID != id {
			shared.Error(w, 400, "PROJECT_MISMATCH")
			return
		}
		if !validHermesSetup(c) {
			shared.Error(w, 400, "INVALID_HERMES_SETUP")
			return
		}
		if c.ConnectionProfileID != "" {
			p, err := s.Store.ConnectionProfile(r.Context(), c.ConnectionProfileID)
			if err != nil || p.Policy.Mode == "open" {
				shared.Error(w, 400, "INVALID_HERMES_CONNECTION_PROFILE")
				return
			}
		}
		previous := c.Revision
		c.Revision++
		now := s.now()
		c.UpdatedAt = &now
		err := s.Store.SaveHermesSetup(r.Context(), c, previous)
		if errors.Is(err, store.ErrHermesRevision) {
			shared.Error(w, 409, "HERMES_REVISION_CONFLICT")
			return
		}
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, s.hermesResponse(r.Context(), c))
	})
}
