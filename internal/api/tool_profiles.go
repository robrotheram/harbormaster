package api

import (
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) registerToolProfiles(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/tool-profiles", func(w http.ResponseWriter, r *http.Request) {
		profiles, err := s.Store.ToolProfiles(r.Context())
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
			Title       string   `json:"title"`
			Description string   `json:"description"`
			MCPs        []string `json:"mcps"`
			Revision    int      `json:"revision"`
		}
		if !shared.Decode(w, r, &input, 32768) {
			return
		}
		input.Title = strings.TrimSpace(input.Title)
		if input.Title == "" || len(input.Title) > 80 || len(input.Description) > 500 || strings.ContainsAny(input.Title+input.Description, "\r\n\x00") || !shared.ValidMCPs(input.MCPs) {
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
			id = shared.NewID("tp_")
			status = 201
		} else if !shared.ValidID(id) || expected < 1 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if input.MCPs == nil {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		p := shared.Profile{ID: id, Title: input.Title, Description: input.Description, MCPs: append([]string{}, input.MCPs...), Revision: expected + 1, UpdatedAt: s.now()}
		if err := s.Store.SaveToolProfile(r.Context(), p, expected); err != nil {
			if errors.Is(err, store.ErrProfileConflict) {
				shared.Error(w, 409, "PROFILE_REVISION_CONFLICT")
			} else {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}
		shared.JSON(w, status, p)
	}
	mux.HandleFunc("POST /v1/tool-profiles", save)
	mux.HandleFunc("POST /v1/tool-profiles/{id}", save)
}
