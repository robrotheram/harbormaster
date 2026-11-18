package api

import (
	"agent-workbench/internal/shared"
	"net/http"
)

func (s *Server) registerWorkspaceSettings(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/workspaces/{id}/settings", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			IdleTimeoutMinutes  *int                     `json:"idle_timeout_minutes"`
			Connections         *shared.ConnectionPolicy `json:"connections"`
			ConnectionProfileID *string                  `json:"connection_profile_id"`
		}
		if !decodeRouteBody(w, r, &input) {
			return
		}
		hasProfile := input.ConnectionProfileID != nil && *input.ConnectionProfileID != ""
		if input.IdleTimeoutMinutes == nil || (input.Connections == nil && !hasProfile) || (input.Connections != nil && !input.Connections.Valid()) || (*input.IdleTimeoutMinutes != 0 && (*input.IdleTimeoutMinutes < 5 || *input.IdleTimeoutMinutes > 1440)) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock(r.PathValue("id"))
		defer unlock()
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if err := s.refresh(r.Context(), &ws); err != nil {
			apiError(w, err)
			return
		}
		previous := ws
		ws.IdleTimeoutMinutes = input.IdleTimeoutMinutes
		if hasProfile {
			ws.ConnectionProfileID = *input.ConnectionProfileID
			if err := s.resolveConnectionProfile(r.Context(), &ws); err != nil {
				shared.Error(w, 400, "INVALID_CONNECTION_PROFILE")
				return
			}
		} else {
			ws.ConnectionProfileID = ""
			ws.ConnectionProfileRevision = 0
			ws.Connections = input.Connections

		}
		if err := s.checkConnectionTransition(ws); err != nil {
			apiError(w, err)
			return
		}
		now := s.now()
		ws.LastOpenedAt = &now
		ws.UpdatedAt = now
		if ws.State != "STOPPED" {
			if err := s.lifecycle().ConfigureConnections(r.Context(), ws); err != nil {
				apiError(w, err)
				return
			}
		}
		if err := s.refresh(r.Context(), &ws); err != nil {
			_ = s.lifecycle().ConfigureConnections(r.Context(), previous)
			apiError(w, err)
			return
		}
		if err := s.Store.Save(r.Context(), ws); err != nil {
			if ws.State != "STOPPED" {
				_ = s.lifecycle().ConfigureConnections(r.Context(), previous)
			}
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, ws)
	})
}
