package api

import (
	"encoding/json"
	"io"
	"net/http"

	"agent-workbench/internal/shared"
)

func (s *Server) workerSetup(w http.ResponseWriter, r *http.Request) {
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
	var body []byte
	if r.Method == http.MethodPost {
		var err error
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, shared.APIBodyLimit(r.Pattern)))
		if err != nil {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
	}
	observation, err := s.Runtime.Inspect(r.Context(), ws.ID)
	if err != nil || !observation.Ready {
		shared.Error(w, 409, "WORKSPACE_NOT_READY")
		return
	}
	status, out, err := s.Runtime.Agent(r.Context(), ws.ID, r.Method, "/v1/setup", body)
	if err != nil {
		shared.Error(w, 503, "SETUP_UNAVAILABLE")
		return
	}
	if status < 200 || status >= 300 {
		var failure shared.APIError
		_ = json.Unmarshal(out, &failure)
		switch failure.Code {
		case "INVALID_REQUEST", "WORKSPACE_STOPPING", "WORKSPACE_BUSY",
			"SETUP_BUSY", "SETUP_REVIEW_PENDING", "SETUP_UNAVAILABLE",
			"SETUP_UNSUPPORTED_CONFIG", "SETUP_CHECKOUT_CHANGED", "SETUP_SKILL_CONFLICT",
			"SETUP_DOWNLOAD_FAILED", "SETUP_NETWORK_BLOCKED", "SETUP_STORAGE_FAILED", "SETUP_INTERRUPTED":
			shared.Error(w, status, failure.Code)
		default:
			shared.Error(w, 503, "SETUP_UNAVAILABLE")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(out)
}
