package api

import (
	"agent-workbench/internal/previewsvc"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// Factory operations use ordinary project authentication and workspace lifecycle.
// They are intentionally separate from work-attempt projection/dispatch.
func (s *Server) registerFactoryEnvironments(mux *http.ServeMux) {
	for _, resource := range []string{"recipe-jobs", "artifacts"} {
		mux.HandleFunc("GET /v1/workspaces/{id}/"+resource, func(w http.ResponseWriter, r *http.Request) {
			ws, ok := s.workspace(w, r)
			if !ok {
				return
			}
			unlock := s.operations.Lock(ws.ID)
			defer unlock()
			ws, ok = s.workspace(w, r)
			if !ok {
				return
			}
			if err := s.lifecycle().ObserveTask(r.Context(), &ws); err != nil {
				apiError(w, err)
				return
			}
			s.factoryForward(w, r, ws.ID, "/v1/"+resource, nil)
		})
	}
	mux.HandleFunc("GET /v1/workspaces/{id}/artifacts/{artifact}/content", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		id := r.PathValue("artifact")
		if !shared.ValidID(id) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock(ws.ID)
		defer unlock()
		ws, ok = s.workspace(w, r)
		if !ok {
			return
		}
		if err := s.lifecycle().ObserveTask(r.Context(), &ws); err != nil {
			apiError(w, err)
			return
		}
		status, b, err := s.Runtime.Agent(r.Context(), ws.ID, "GET", "/v1/artifacts/"+id+"/content", nil)
		if err != nil || status != 200 {
			shared.Error(w, 503, "EVIDENCE_UNAVAILABLE")
			return
		}
		if len(b) > 1<<20 {
			shared.Error(w, 413, "EVIDENCE_READ_BUDGET_EXCEEDED")
			return
		}
		shared.JSON(w, 200, map[string]any{"artifact_id": id, "content": b})
	})

	mux.HandleFunc("POST /v1/workspaces/{id}/recipe-jobs", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Template string `json:"template"`
			Key      string `json:"idempotency_key"`
			Timeout  int    `json:"timeout_seconds"`
			Resource string `json:"resource_class"`
		}
		if !shared.Decode(w, r, &in, 2048) {
			return
		}
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if (in.Template != "gogallery-v1" && in.Template != "research-v1") || !shared.ValidKey(in.Key) || in.Timeout < 0 || in.Timeout > 3600 || (in.Resource != "" && in.Resource != "cpu-small") {
			shared.Error(w, 422, "UNSUPPORTED_APPROVED_TEMPLATE_OR_BUDGET")
			return
		}
		if in.Timeout == 0 {
			in.Timeout = 600
			if in.Template == "gogallery-v1" {
				in.Timeout = 1800
			}
		}
		unlock := s.operations.Lock(ws.ID)
		defer unlock()
		ws, ok = s.workspace(w, r)
		if !ok {
			return
		}
		if err := s.start(r.Context(), &ws); err != nil {
			apiError(w, err)
			return
		}
		b, _ := json.Marshal(in)
		s.factoryForward(w, r, ws.ID, "/v1/recipe-jobs", b)
	})
	for _, route := range []struct{ pattern, suffix string }{
		{"GET /v1/workspaces/{id}/recipe-jobs/{job}", ""},
		{"POST /v1/workspaces/{id}/recipe-jobs/{job}/cancel", "/cancel"},
	} {
		mux.HandleFunc(route.pattern, func(w http.ResponseWriter, r *http.Request) {
			ws, ok := s.workspace(w, r)
			if !ok {
				return
			}
			job := r.PathValue("job")
			if !shared.ValidID(job) {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			unlock := s.operations.Lock(ws.ID)
			defer unlock()
			ws, ok = s.workspace(w, r)
			if !ok {
				return
			}
			if err := s.lifecycle().ObserveTask(r.Context(), &ws); err != nil {
				apiError(w, err)
				return
			}
			s.factoryForward(w, r, ws.ID, "/v1/recipe-jobs/"+job+route.suffix, []byte(`{}`))
		})
	}
	mux.HandleFunc("POST /v1/workspaces/{id}/retained-previews", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Retained string `json:"retained_workspace_id"`
			Key      string `json:"idempotency_key"`
			Artifact string `json:"artifact_id"`
			TTL      int    `json:"ttl_seconds"`
		}
		if !shared.Decode(w, r, &in, 2048) {
			return
		}
		coding, ok := s.workspace(w, r)
		if !ok {
			return
		}
		retained, err := s.Store.Workspace(r.Context(), in.Retained)
		if err != nil || retained.ProjectID != coding.ProjectID {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if !s.AuthorizeProject(w, r, retained.ProjectID) {
			return
		}
		unlock := s.operations.Lock(coding.ID, retained.ID)
		defer unlock()
		coding, ok = s.workspace(w, r)
		if !ok {
			return
		}
		retained, err = s.Store.Workspace(r.Context(), in.Retained)
		if err != nil || retained.ProjectID != coding.ProjectID {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		svc := previewsvc.Service{Runtime: s.Runtime, Lifecycle: s.lifecycle()}
		if in.Key == "" {
			in.Key = "retain-" + coding.ID + "-" + in.Artifact
		}
		body, err := svc.Retain(r.Context(), coding, &retained, in.Artifact, in.TTL, in.Key)
		if err != nil {
			if errors.Is(err, previewsvc.ErrConflict) {
				shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
				return
			}
			shared.Error(w, 503, "RETAINED_PREVIEW_UNAVAILABLE")
			return
		}
		var service struct {
			ID        string `json:"id"`
			ExpiresAt int64  `json:"expires_at"`
		}
		if json.Unmarshal(body, &service) != nil || !shared.ValidID(service.ID) || service.ExpiresAt <= 0 {
			shared.Error(w, 503, "INVALID_SERVICE_RESPONSE")
			return
		}
		retained.RetainedPreview = &shared.RetainedPreview{ServiceID: service.ID, SourceWorkspaceID: coding.ID, ArtifactID: in.Artifact, ExpiresAt: service.ExpiresAt}
		if err := s.Store.Save(r.Context(), retained); err != nil {
			shared.Error(w, 503, "RETENTION_METADATA_UNAVAILABLE")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(201)
		w.Write(body)
	})
	mux.HandleFunc("POST /v1/workspaces/{id}/services/{service}/release", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		id := r.PathValue("service")
		if !shared.ValidID(id) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock(ws.ID)
		defer unlock()
		ws, ok = s.workspace(w, r)
		if !ok {
			return
		}
		svc := previewsvc.Service{Runtime: s.Runtime, Lifecycle: s.lifecycle()}
		if ws.State != "STOPPED" {
			if err := svc.ReleaseExpired(r.Context(), &ws, id); err != nil {
				shared.Error(w, 409, "PREVIEW_RELEASE_NOT_CONFIRMED")
				return
			}
		}
		shared.JSON(w, 200, map[string]string{"workspace_id": ws.ID, "state": ws.State})
	})
	for _, route := range []struct{ pattern, suffix string }{
		{"GET /v1/workspaces/{id}/services/{service}", ""},
		{"POST /v1/workspaces/{id}/services/{service}/fetch", "/fetch"},
		{"POST /v1/workspaces/{id}/services/{service}/revoke", "/revoke"},
	} {
		mux.HandleFunc(route.pattern, func(w http.ResponseWriter, r *http.Request) {
			ws, ok := s.workspace(w, r)
			if !ok {
				return
			}
			id := r.PathValue("service")
			if !shared.ValidID(id) {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			var b []byte
			if route.suffix == "/fetch" {
				var in struct {
					Path string `json:"path"`
				}
				if !shared.Decode(w, r, &in, 2048) {
					return
				}
				if !shared.ValidProjectFile(in.Path) {
					shared.Error(w, 400, "INVALID_REQUEST")
					return
				}
				b, _ = json.Marshal(in)
			}
			// Reads/revocation never restart stopped preview compute.
			s.factoryForward(w, r, ws.ID, "/v1/services/"+id+route.suffix, b)
		})
	}
}
func (s *Server) factoryForward(w http.ResponseWriter, r *http.Request, id, path string, b []byte) {
	status, out, err := s.Runtime.Agent(r.Context(), id, r.Method, path, b)
	if err != nil || !json.Valid(out) || status < 200 || status > 599 {
		shared.Error(w, 503, "RUNTIME_UNAVAILABLE")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	w.Write(out)
}

// Called under the existing workspace lock by the ordinary durable reconciler.
func (s *Server) reconcileRetainedPreview(ctx context.Context, ws *shared.Workspace) {
	if ws.RetainedPreview == nil || ws.State == "STOPPED" || s.now().Unix() < ws.RetainedPreview.ExpiresAt {
		return
	}
	svc := previewsvc.Service{Runtime: s.Runtime, Lifecycle: s.lifecycle()}
	// Unknown activity, service status or failed termination retains capacity.
	_ = svc.ReleaseExpired(ctx, ws, ws.RetainedPreview.ServiceID)
}
