package api

import (
	"agent-workbench/internal/coordination"
	"agent-workbench/internal/shared"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// RegisterCoordination is wired by the composition root after installing Schema.
// Project authorization applies to every call, including both grant endpoints.
// Notes must resolve only explicitly configured project/workspace integrations.
func (s *Server) RegisterCoordination(mux *http.ServeMux, service *coordination.Service, notes func(string, string) (coordination.NotePublisher, error)) {
	reply := func(w http.ResponseWriter, v any, e error) {
		if e != nil {
			status := 503
			code := "COORDINATION_UNAVAILABLE"
			if errors.Is(e, coordination.ErrDenied) {
				status = 403
				code = "EXCHANGE_DENIED"
			}
			if errors.Is(e, coordination.ErrConflict) {
				status = 409
				code = "PUBLICATION_CONFLICT"
			}
			if errors.Is(e, coordination.ErrInvalid) {
				status = 400
				code = "INVALID_REQUEST"
			}
			shared.Error(w, status, code)
			return
		}
		shared.JSON(w, 200, v)
	}
	mux.HandleFunc("POST /v1/project-exchanges", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		var in coordination.Grant
		if !shared.Decode(w, r, &in, 4096) || !s.AuthorizeProject(w, r, in.Source) || !s.AuthorizeProject(w, r, in.Destination) {
			return
		}
		reply(w, in, service.Create(r.Context(), in.Source, in))
	})
	for _, action := range []string{"approve", "revoke"} {
		mux.HandleFunc("POST /v1/project-exchanges/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			if !s.admin(w, r) {
				return
			}
			var in struct {
				ProjectID string `json:"project_id"`
			}
			if !shared.Decode(w, r, &in, 1024) || !s.AuthorizeProject(w, r, in.ProjectID) {
				return
			}
			var e error
			if action == "approve" {
				e = service.Approve(r.Context(), in.ProjectID, r.PathValue("id"))
			} else {
				e = service.Revoke(r.Context(), in.ProjectID, r.PathValue("id"))
			}
			reply(w, map[string]bool{"ok": e == nil}, e)
		})
	}
	mux.HandleFunc("POST /v1/project-exchanges/{id}/exports", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID  string `json:"project_id"`
			Occurrence string `json:"occurrence"`
			Body       string `json:"body"`
		}
		if !shared.Decode(w, r, &in, 70000) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		x, e := service.Publish(r.Context(), in.ProjectID, r.PathValue("id"), in.Occurrence, []byte(in.Body))
		reply(w, x, e)
	})
	mux.HandleFunc("GET /v1/exchange-inbox", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			var err error
			limit, err = strconv.Atoi(v)
			if err != nil {
				reply(w, nil, coordination.ErrInvalid)
				return
			}
		}
		page, err := service.Inbox(r.Context(), project, r.URL.Query().Get("after"), limit)
		reply(w, page, err)
	})
	mux.HandleFunc("GET /v1/exchange-exports/{id}", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		x, b, e := service.Read(r.Context(), project, r.PathValue("id"))
		reply(w, map[string]any{"export": x, "body": string(b)}, e)
	})
	mux.HandleFunc("POST /v1/exchange-exports/{id}/submit", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !shared.Decode(w, r, &in, 1024) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		var response *coordinationResponse
		e := service.Consume(r.Context(), in.ProjectID, r.PathValue("id"), func(body []byte, key string) (string, error) {
			var spec struct {
				WorkspaceID   string   `json:"workspace_id"`
				Path          string   `json:"specification_path"`
				Digest        string   `json:"specification_digest"`
				Kind          string   `json:"kind"`
				ArtifactPaths []string `json:"artifact_paths"`
			}
			d := json.NewDecoder(bytes.NewReader(body))
			d.DisallowUnknownFields()
			if d.Decode(&spec) != nil {
				return "", coordination.ErrInvalid
			}
			var extra any
			if d.Decode(&extra) != io.EOF {
				return "", coordination.ErrInvalid
			}
			payload, _ := json.Marshal(map[string]any{"project_id": in.ProjectID, "workspace_id": spec.WorkspaceID, "specification_path": spec.Path, "specification_digest": spec.Digest, "kind": spec.Kind, "idempotency_key": key, "artifact_paths": spec.ArtifactPaths})
			request := r.Clone(r.Context())
			u := *r.URL
			u.Path = "/v1/work-items"
			u.RawQuery = ""
			request.URL = &u
			request.Body = io.NopCloser(bytes.NewReader(payload))
			request.ContentLength = int64(len(payload))
			response = &coordinationResponse{headers: make(http.Header)}
			mux.ServeHTTP(response, request)
			if response.Code < 200 || response.Code >= 300 {
				return "", fmt.Errorf("work creation failed: %d", response.Code)
			}
			var work struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &work); err != nil {
				return "", err
			}
			return work.ID, nil
		})
		if response != nil && (e == nil || response.Code >= 300) {
			for k, values := range response.Header() {
				w.Header()[k] = values
			}
			w.WriteHeader(response.Code)
			_, _ = w.Write(response.Body.Bytes())
			return
		}
		reply(w, nil, e)
	})

	mux.HandleFunc("POST /v1/note-publications", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID   string                 `json:"project_id"`
			WorkspaceID string                 `json:"workspace_id"`
			Write       coordination.NoteWrite `json:"write"`
		}
		if !shared.Decode(w, r, &in, 70000) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		ws, e := s.Store.Workspace(r.Context(), in.WorkspaceID)
		if e != nil || ws.ProjectID != in.ProjectID {
			reply(w, nil, coordination.ErrDenied)
			return
		}
		n, e := notes(in.ProjectID, in.WorkspaceID)
		if e != nil {
			reply(w, nil, e)
			return
		}
		receipt, e := n.Publish(in.Write)
		reply(w, receipt, e)
	})
}

// Capture the fixed work-create adapter response before committing a consumption
// receipt. A failed factory operation must remain retryable in the inbox.
type coordinationResponse struct {
	headers http.Header
	Code    int
	Body    bytes.Buffer
}

func (r *coordinationResponse) Header() http.Header { return r.headers }
func (r *coordinationResponse) WriteHeader(status int) {
	if r.Code == 0 {
		r.Code = status
	}
}
func (r *coordinationResponse) Write(body []byte) (int, error) {
	if r.Code == 0 {
		r.Code = http.StatusOK
	}
	return r.Body.Write(body)
}
