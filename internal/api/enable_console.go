package api

import (
	"net/http"

	"agent-workbench/internal/shared"
)

func (s *Server) registerConsoleUpgrade(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/workspaces/{id}/enable-t3", func(w http.ResponseWriter, r *http.Request) {
		if !mutationKey(w, r) {
			return
		}
		unlock := s.operations.Lock(r.PathValue("id"))
		defer unlock()
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if ws.AgentKind != "codex" && ws.AgentKind != "t3" {
			shared.Error(w, 409, "AGENT_PROTOCOL_UNAVAILABLE")
			return
		}
		if err := s.start(r.Context(), &ws); err != nil {
			apiError(w, err)
			return
		}
		status, _, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/console/enable", nil)
		if err != nil || status != 200 {
			if status == 409 {
				shared.Error(w, 409, "WORKSPACE_BUSY")
			} else {
				shared.Error(w, 503, "CONSOLE_UPGRADE_REQUIRED")
			}
			return
		}
		ws.ConsoleEnabled = true
		if s.Store.Save(r.Context(), ws) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, ws)
	})
}
