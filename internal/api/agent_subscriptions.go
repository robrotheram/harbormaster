package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"agent-workbench/internal/subscriptions"
)

func (s *Server) registerAgentSubscriptions(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		owner := s.subscriptionUser(w, r)
		if owner == "" {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		saved, err := s.Store.AgentSubscriptions(r.Context(), owner)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		items := []map[string]any{}
		for _, v := range saved {
			c, err := subscriptions.Open(s.cfg.Token, subscriptions.Binding(owner, v.ID), v.Credential)
			if err != nil {
				shared.Error(w, 503, "CREDENTIAL_UNAVAILABLE")
				return
			}
			workers, err := s.Store.AgentSubscriptionWorkers(r.Context(), owner, v.ID)
			if err != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			items = append(items, map[string]any{"id": v.ID, "name": v.Name, "provider": v.Provider, "account_id": c.AccountID, "expires_at": c.ExpiresAt, "expired": c.ExpiresAt <= s.now().Add(time.Minute).Unix(), "workers": workers})
		}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
	save := func(w http.ResponseWriter, r *http.Request) {
		owner := s.subscriptionUser(w, r)
		if owner == "" {
			return
		}
		var input struct {
			Name     string          `json:"name"`
			Provider string          `json:"provider"`
			Auth     json.RawMessage `json:"auth"`
		}
		if !shared.Decode(w, r, &input, 70<<10) {
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if len(input.Name) == 0 || len(input.Name) > 80 || strings.ContainsFunc(input.Name, unicode.IsControl) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("subscription:" + owner)
		defer unlock()
		id := r.PathValue("subscription")
		create := id == ""
		var v store.AgentSubscription
		if create {
			if !subscriptions.ValidProvider(input.Provider) {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			saved, err := s.Store.AgentSubscriptions(r.Context(), owner)
			if err != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			if len(saved) >= 50 {
				shared.Error(w, 409, "SUBSCRIPTION_LIMIT_REACHED")
				return
			}
			v = store.AgentSubscription{ID: shared.NewID("sub_"), Provider: input.Provider}
		} else {
			if !shared.ValidID(id) {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			var err error
			v, err = s.Store.AgentSubscription(r.Context(), owner, id)
			if errors.Is(err, sql.ErrNoRows) {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
			if err != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			if input.Provider != "" && input.Provider != v.Provider {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
		}
		v.Name = input.Name
		if create || len(input.Auth) > 0 {
			c, err := subscriptions.ParseProvider(v.Provider, input.Auth, s.now())
			if err != nil {
				shared.Error(w, 400, "SUBSCRIPTION_INVALID")
				return
			}
			v.Credential, err = subscriptions.Seal(s.cfg.Token, subscriptions.Binding(owner, v.ID), c)
			if err != nil {
				shared.Error(w, 503, "CREDENTIAL_UNAVAILABLE")
				return
			}
		}
		if err := s.Store.SaveAgentSubscription(r.Context(), owner, v, create); err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"id": v.ID, "saved": true})
	}
	mux.HandleFunc("POST /v1/subscriptions", save)
	mux.HandleFunc("POST /v1/subscriptions/{subscription}", save)
	mux.HandleFunc("POST /v1/subscriptions/{subscription}/delete", func(w http.ResponseWriter, r *http.Request) {
		owner := s.subscriptionUser(w, r)
		if owner == "" {
			return
		}
		id := r.PathValue("subscription")
		if !shared.ValidID(id) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("subscription:" + owner)
		defer unlock()
		err := s.Store.DeleteAgentSubscription(r.Context(), owner, id)
		if errors.Is(err, sql.ErrNoRows) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]bool{"deleted": true})
	})
}
