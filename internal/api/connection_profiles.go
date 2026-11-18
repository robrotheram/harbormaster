package api

import (
	"agent-workbench/internal/lifecycle"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"context"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) resolveConnectionProfile(ctx context.Context, w *shared.Workspace) error {
	return s.lifecycle().ResolveConnectionProfile(ctx, w)
}
func (s *Server) checkConnectionTransition(w shared.Workspace) error {
	return lifecycle.CheckConnections(w)
}
func (s *Server) registerConnectionProfiles(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/connection-profiles", func(w http.ResponseWriter, r *http.Request) {
		profiles, err := s.Store.ConnectionProfiles(r.Context())
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": profiles})
	})
	save := func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		var input struct {
			Name        string                  `json:"name"`
			Description string                  `json:"description"`
			Policy      shared.ConnectionPolicy `json:"policy"`
			Revision    int                     `json:"revision"`
		}
		if !decodeRouteBody(w, r, &input) {
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || len(input.Name) > 80 || len(input.Description) > 500 || strings.ContainsAny(input.Name+input.Description, "\r\n\x00") || !input.Policy.Valid() {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		id := r.PathValue("id")
		expected := input.Revision
		status := 200
		if id == "" {
			if expected != 0 {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			id = shared.NewID("np_")
			status = 201
		} else if !shared.ValidID(id) || expected < 1 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		p := shared.ConnectionProfile{ID: id, Name: input.Name, Description: input.Description, Policy: input.Policy, Revision: expected + 1, UpdatedAt: s.now()}
		if err := s.Store.SaveConnectionProfile(r.Context(), p, expected); err != nil {
			if errors.Is(err, store.ErrProfileConflict) {
				shared.Error(w, 409, "PROFILE_REVISION_CONFLICT")
			} else {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}
		shared.JSON(w, status, p)
	}
	mux.HandleFunc("POST /v1/connection-profiles/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		var input struct {
			Revision int `json:"revision"`
		}
		if !decodeRouteBody(w, r, &input) {
			return
		}
		id := r.PathValue("id")
		if !shared.ValidID(id) || input.Revision < 1 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if err := s.Store.DeleteConnectionProfile(r.Context(), id, input.Revision); err != nil {
			switch {
			case errors.Is(err, store.ErrProfileInUse):
				shared.Error(w, 409, "CONNECTION_PROFILE_IN_USE")
			case errors.Is(err, store.ErrProfileConflict):
				shared.Error(w, 409, "PROFILE_REVISION_CONFLICT")
			default:
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}
		shared.JSON(w, 200, map[string]bool{"deleted": true})
	})
	mux.HandleFunc("POST /v1/connection-profiles", save)
	mux.HandleFunc("POST /v1/connection-profiles/{id}", save)
}
