package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
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
			var input struct {
				ID       string `json:"subscription_id"`
				Provider string `json:"provider"`
			}
			if !decodeRouteBody(w, r, &input) {
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
				if ws.AgentKind == "hermes" && input.Provider != "codex" {
					shared.Error(w, 409, "PROVIDER_NOT_CONFIGURED")
					return
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
				if ws.AgentKind == "hermes" && saved.Provider != "codex" {
					shared.Error(w, 409, "PROVIDER_NOT_CONFIGURED")
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
	if !decodeRouteBody(w, r, &input) {
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
	c, err := s.workerSubscription(r.Context(), input.WorkspaceID, input.Provider)
	if err != nil {
		shared.Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
		return
	}
	shared.JSON(w, 200, c)
}

func (s *Server) workerSubscription(ctx context.Context, workspace, provider string) (subscriptions.Credential, error) {
	owner, id, err := s.Store.WorkerAgentSubscription(ctx, workspace, provider)
	if err != nil {
		return subscriptions.Credential{}, err
	}
	return s.validSubscription(ctx, owner, id, provider)
}

var subscriptionRequired = shared.CodedError{Status: http.StatusConflict, Code: "PROVIDER_LOGIN_REQUIRED"}

func (s *Server) validSubscription(ctx context.Context, owner, id, provider string) (subscriptions.Credential, error) {
	saved, err := s.Store.AgentSubscription(ctx, owner, id)
	if err != nil || saved.Provider != provider {
		return subscriptions.Credential{}, subscriptionRequired
	}
	c, err := subscriptions.Open(s.cfg.Token, subscriptions.Binding(owner, id), saved.Credential)
	if err != nil || c.ExpiresAt <= s.now().Add(time.Minute).Unix() {
		return subscriptions.Credential{}, subscriptionRequired
	}
	return c, nil
}

// An explicit worker binding wins. A personal default only fills an empty slot
// during an authorized human run, so background work never borrows a credential.
func (s *Server) prepareWorkSubscription(r *http.Request, workspace, provider string) error {
	if s.cfg.SubscriptionGatewayToken == "" || workspace == "" {
		return nil
	}
	if provider == "" || provider == "hermes" {
		provider = "codex"
	}
	ctx := r.Context()
	if _, err := s.workerSubscription(ctx, workspace, provider); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		if errors.Is(err, subscriptionRequired) {
			return subscriptionRequired
		}
		return err
	}
	actor, _ := ctx.Value(principalKey{}).(principal)
	if actor.human.ID == "" {
		return subscriptionRequired
	}
	defaults, err := s.Store.AgentSubscriptionDefaults(ctx, actor.human.ID)
	if err != nil {
		return err
	}
	id := defaults[provider]
	if id == "" {
		return subscriptionRequired
	}
	if _, err = s.validSubscription(ctx, actor.human.ID, id, provider); err != nil {
		return err
	}
	if err = s.Store.LoadDefaultAgentSubscription(ctx, actor.human.ID, id, provider, workspace); err != nil {
		return err
	}
	if _, err = s.workerSubscription(ctx, workspace, provider); err != nil {
		return subscriptionRequired
	}
	return nil
}
