package api

import (
	"encoding/json"
	"io"
	"net/http"

	"agent-workbench/internal/shared"
)

func (s *Server) registerTerminals(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/workspaces/{id}/terminals", s.terminalRoute)
	mux.HandleFunc("POST /v1/workspaces/{id}/terminals/{terminal}/{action}", s.terminalRoute)
}

func (s *Server) terminalRoute(w http.ResponseWriter, r *http.Request) {
	ws, ok := s.workspace(w, r)
	if !ok {
		return
	}
	if ws.Shared {
		if !s.admin(w, r) {
			return
		}
	} else if !s.projectAdmin(w, r, ws.ProjectID) {
		return
	}
	if r.URL.RawQuery != "" {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	path := "/v1/terminals"
	if id := r.PathValue("terminal"); id != "" {
		action := r.PathValue("action")
		if !shared.ValidID(id) || (action != "exchange" && action != "close") {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		path += "/" + id + "/" + action
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, shared.APIBodyLimit(r.Pattern)))
	if err != nil {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	// Never start compute as a side effect of opening or polling a terminal.
	if r.PathValue("terminal") == "" {
		observation, err := s.Runtime.Inspect(r.Context(), ws.ID)
		if err != nil || !observation.Ready {
			shared.Error(w, 409, "WORKSPACE_NOT_READY")
			return
		}
	}
	status, out, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", path, body)
	if err != nil {
		shared.Error(w, 503, "TERMINAL_UNAVAILABLE")
		return
	}
	if status == 404 || status == 405 {
		shared.Error(w, 409, "TERMINAL_UNAVAILABLE")
		return
	}
	if status < 200 || status >= 300 {
		var failure shared.APIError
		_ = json.Unmarshal(out, &failure)
		switch failure.Code {
		case "INVALID_REQUEST", "WORKSPACE_STOPPING", "TERMINAL_EXPIRED", "TERMINAL_LIMIT", "TERMINAL_UNAVAILABLE":
			shared.Error(w, status, failure.Code)
		default:
			shared.Error(w, 503, "TERMINAL_UNAVAILABLE")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(out)
}
