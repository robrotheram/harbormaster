package api

import (
	"agent-workbench/internal/shared"
	"encoding/json"
	"net/http"
)

type applicationRuntime interface {
	Application(http.ResponseWriter, *http.Request, string, string)
}

func (s *Server) registerApplications(mux *http.ServeMux) {
	mux.HandleFunc("/v1/workspaces/{id}/application-proxy/{service}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		service := r.PathValue("service")
		if !shared.ValidID(service) {
			shared.Error(w, 404, "SERVICE_NOT_FOUND")
			return
		}
		runtime, ok := s.Runtime.(applicationRuntime)
		if !ok {
			shared.Error(w, 503, "APPLICATION_PROXY_UNAVAILABLE")
			return
		}
		runtime.Application(w, r, ws.ID, service+"/"+r.PathValue("path"))
	})
	mux.HandleFunc("GET /v1/workspaces/{id}/ports", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if err := s.refresh(r.Context(), &ws); err != nil {
			apiError(w, err)
			return
		}
		if !ws.Ready {
			shared.JSON(w, 200, map[string]any{"items": []any{}, "workspace_stopped": true})
			return
		}
		s.factoryForward(w, r, ws.ID, "/v1/ports", nil)
	})
	for _, method := range []string{"GET", "POST"} {
		mux.HandleFunc(method+" /v1/workspaces/{id}/applications", func(w http.ResponseWriter, r *http.Request) {
			ws, ok := s.workspace(w, r)
			if !ok {
				return
			}
			unlock := s.operations.Lock(ws.ID)
			defer unlock()
			var body []byte
			if r.Method == "POST" {
				var in struct {
					Application shared.ApplicationConfig `json:"application"`
					Key         string                   `json:"idempotency_key"`
					TTL         int                      `json:"ttl_seconds"`
				}
				if !decodeRouteBody(w, r, &in) {
					return
				}
				if !in.Application.Valid() || !shared.ValidKey(in.Key) || in.TTL < 1 || in.TTL > 86400 {
					shared.Error(w, 400, "INVALID_APPLICATION")
					return
				}
				if err := s.start(r.Context(), &ws); err != nil {
					apiError(w, err)
					return
				}
				body, _ = json.Marshal(in)
			} else {
				if err := s.refresh(r.Context(), &ws); err != nil {
					apiError(w, err)
					return
				}
				if !ws.Ready {
					shared.JSON(w, 200, map[string]any{"items": []any{}, "workspace_stopped": true})
					return
				}
			}
			s.factoryForward(w, r, ws.ID, "/v1/services", body)
		})
	}
}
