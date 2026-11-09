package api

import (
	"agent-workbench/internal/shared"
	"context"
	"net/http"
)

func (s *Server) configureNetwork(ctx context.Context, w shared.Workspace) error {
	return s.lifecycle().ConfigureNetwork(ctx, w)
}
func (s *Server) registerWorkspaceSettings(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/workspaces/{id}/settings", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			IdleTimeoutMinutes  *int                     `json:"idle_timeout_minutes"`
			GitHubAccess        *bool                    `json:"github_access"`
			Connections         *shared.ConnectionPolicy `json:"connections"`
			ConnectionProfileID *string                  `json:"connection_profile_id"`
		}
		if !shared.Decode(w, r, &input, 32768) {
			return
		}
		if input.IdleTimeoutMinutes == nil || (input.GitHubAccess == nil && input.Connections == nil && input.ConnectionProfileID == nil) || (input.Connections != nil && !input.Connections.Valid()) || (*input.IdleTimeoutMinutes != 0 && (*input.IdleTimeoutMinutes < 5 || *input.IdleTimeoutMinutes > 1440)) {
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
		if input.ConnectionProfileID != nil && *input.ConnectionProfileID != "" {
			ws.ConnectionProfileID = *input.ConnectionProfileID
			if err := s.resolveConnectionProfile(r.Context(), &ws); err != nil {
				shared.Error(w, 400, "INVALID_CONNECTION_PROFILE")
				return
			}
		} else if input.Connections != nil {
			ws.ConnectionProfileID = ""
			ws.ConnectionProfileRevision = 0
			ws.Connections = input.Connections
			ws.GitHubAccess = input.Connections.Allows("github.com:443")
		} else {
			if input.GitHubAccess == nil {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			ws.ConnectionProfileID = ""
			ws.ConnectionProfileRevision = 0
			ws.Connections = nil
			ws.GitHubAccess = *input.GitHubAccess
		}
		if err := s.checkConnectionTransition(ws); err != nil {
			apiError(w, err)
			return
		}
		now := s.now()
		ws.LastOpenedAt = &now
		ws.UpdatedAt = now
		if ws.State != "STOPPED" {
			if err := s.configureNetwork(r.Context(), ws); err != nil {
				apiError(w, err)
				return
			}
		}
		if err := s.refresh(r.Context(), &ws); err != nil {
			_ = s.configureNetwork(r.Context(), previous)
			apiError(w, err)
			return
		}
		if err := s.Store.Save(r.Context(), ws); err != nil {
			if ws.State != "STOPPED" {
				_ = s.configureNetwork(r.Context(), previous)
			}
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, ws)
	})
}
