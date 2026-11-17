package api

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"agent-workbench/internal/subscriptions"
)

const subscriptionUserHeader = "X-Workbench-Subscription-User"

func (s *Server) subscriptionUser(w http.ResponseWriter, r *http.Request) string {
	p, _ := r.Context().Value(principalKey{}).(principal)
	if p.human.ID != "" {
		return p.human.ID
	}
	if !s.admin(w, r) {
		return ""
	}
	user := r.Header.Get(subscriptionUserHeader)
	if !strings.HasPrefix(user, "user_") || !shared.ValidID(user) {
		shared.Error(w, 401, "AUTH_IDENTITY_UNAVAILABLE")
		return ""
	}
	return user
}
func (s *Server) registerSubscriptions(mux *http.ServeMux) {
	s.registerAgentSubscriptions(mux)
	mux.HandleFunc("GET /v1/subscription", func(w http.ResponseWriter, r *http.Request) {
		owner := s.subscriptionUser(w, r)
		if owner == "" {
			return
		}
		sealed, err := s.Store.Subscription(r.Context(), owner)
		if errors.Is(err, sql.ErrNoRows) {
			shared.JSON(w, 200, map[string]any{"saved": false, "workers": []string{}})
			return
		}
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		c, err := subscriptions.Open(s.cfg.Token, owner, sealed)
		if err != nil {
			shared.Error(w, 503, "CREDENTIAL_UNAVAILABLE")
			return
		}
		workers, err := s.Store.SubscriptionWorkers(r.Context(), owner)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"saved": true, "account_id": c.AccountID, "expires_at": c.ExpiresAt, "expired": c.ExpiresAt <= s.now().Add(time.Minute).Unix(), "workers": workers})
	})
	mux.HandleFunc("POST /v1/subscription", func(w http.ResponseWriter, r *http.Request) {
		owner := s.subscriptionUser(w, r)
		if owner == "" {
			return
		}
		var input struct {
			Auth json.RawMessage `json:"auth"`
		}
		if !shared.Decode(w, r, &input, 70<<10) {
			return
		}
		c, err := subscriptions.Parse(input.Auth, s.now())
		if err != nil {
			shared.Error(w, 400, "SUBSCRIPTION_INVALID")
			return
		}
		sealed, err := subscriptions.Seal(s.cfg.Token, owner, c)
		if err != nil {
			shared.Error(w, 503, "CREDENTIAL_UNAVAILABLE")
			return
		}
		unlock := s.operations.Lock("subscription:" + owner)
		defer unlock()
		if err = s.Store.SaveSubscription(r.Context(), owner, sealed); err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]bool{"saved": true})
	})
	mux.HandleFunc("POST /v1/subscription/delete", func(w http.ResponseWriter, r *http.Request) {
		owner := s.subscriptionUser(w, r)
		if owner == "" {
			return
		}
		unlock := s.operations.Lock("subscription:" + owner)
		defer unlock()
		if err := s.Store.DeleteSubscription(r.Context(), owner); err != nil && !errors.Is(err, sql.ErrNoRows) {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]bool{"saved": false})
	})
	for _, action := range []string{"load", "unload"} {
		mux.HandleFunc("POST /v1/workspaces/{id}/subscription/"+action, func(w http.ResponseWriter, r *http.Request) {
			owner := s.subscriptionUser(w, r)
			if owner == "" {
				return
			}
			unlock := s.operations.Lock("subscription:" + owner)
			defer unlock()
			unlockWorker := s.operations.Lock(r.PathValue("id"))
			defer unlockWorker()
			ws, ok := s.workspace(w, r)
			if !ok {
				return
			}
			if ws.AgentKind == "hermes" {
				shared.Error(w, 409, "PROVIDER_NOT_CONFIGURED")
				return
			}
			var input struct {
				ID       string `json:"subscription_id"`
				Provider string `json:"provider"`
			}
			if !shared.Decode(w, r, &input, 4096) {
				return
			}
			if input.ID == "" {
				input.ID = "default"
			}
			if !shared.ValidID(input.ID) || (input.Provider != "" && !subscriptions.ValidProvider(input.Provider)) {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			if action == "unload" {
				if input.Provider == "" {
					input.Provider = "codex"
				}
				if err := s.Store.UnloadAgentSubscription(r.Context(), owner, input.Provider, ws.ID); err != nil {
					shared.Error(w, 503, "STORAGE_UNAVAILABLE")
					return
				}
			} else {
				if s.cfg.SubscriptionGatewayToken == "" {
					shared.Error(w, 503, "SUBSCRIPTION_NOT_CONFIGURED")
					return
				}
				saved, err := s.Store.AgentSubscription(r.Context(), owner, input.ID)
				if err != nil {
					shared.Error(w, 409, "PROVIDER_LOGIN_REQUIRED")
					return
				}
				if input.Provider != "" && input.Provider != saved.Provider {
					shared.Error(w, 400, "INVALID_REQUEST")
					return
				}
				c, err := subscriptions.Open(s.cfg.Token, subscriptions.Binding(owner, saved.ID), saved.Credential)
				if err != nil || c.ExpiresAt <= s.now().Add(time.Minute).Unix() {
					shared.Error(w, 409, "PROVIDER_LOGIN_REQUIRED")
					return
				}
				if err = s.Store.LoadAgentSubscription(r.Context(), owner, saved.ID, saved.Provider, ws.ID); err != nil {
					if errors.Is(err, store.ErrSubscriptionInUse) {
						shared.Error(w, 409, "SUBSCRIPTION_IN_USE")
					} else {
						shared.Error(w, 503, "STORAGE_UNAVAILABLE")
					}
					return
				}
			}
			shared.JSON(w, 200, map[string]bool{"loaded": action == "load"})
		})
	}
}

// Only the inference gateway's separate service credential can resolve a
// worker binding. This route is absent from the browser and project MCP surface.
func (s *Server) resolveSubscription(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || s.cfg.SubscriptionGatewayToken == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.SubscriptionGatewayToken)) != 1 {
		shared.Error(w, 401, "UNAUTHORIZED")
		return
	}
	var input struct {
		WorkspaceID string `json:"workspace_id"`
		Provider    string `json:"provider"`
	}
	if !shared.Decode(w, r, &input, 4096) {
		return
	}
	if input.Provider == "" {
		input.Provider = "codex"
	}
	if !subscriptions.ValidProvider(input.Provider) || !shared.ValidID(input.WorkspaceID) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	if _, err := s.Store.Workspace(r.Context(), input.WorkspaceID); err != nil {
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return
	}
	owner, id, err := s.Store.WorkerAgentSubscription(r.Context(), input.WorkspaceID, input.Provider)
	if err != nil {
		shared.Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
		return
	}
	saved, err := s.Store.AgentSubscription(r.Context(), owner, id)
	if err != nil || saved.Provider != input.Provider {
		shared.Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
		return
	}
	c, err := subscriptions.Open(s.cfg.Token, subscriptions.Binding(owner, id), saved.Credential)
	if err != nil || c.ExpiresAt <= s.now().Add(30*time.Second).Unix() {
		shared.Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
		return
	}
	shared.JSON(w, 200, c)
}
