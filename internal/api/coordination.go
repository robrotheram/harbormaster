package api

import (
	"agent-workbench/internal/coordination"
	"agent-workbench/internal/shared"
	"bytes"
	"encoding/json"
	"errors"
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
			var response error
			switch {
			case errors.Is(e, coordination.ErrInvalid):
				response = shared.CodedError{Status: http.StatusBadRequest, Code: "INVALID_REQUEST"}
			case errors.Is(e, coordination.ErrConflict):
				response = shared.CodedError{Status: http.StatusConflict, Code: "PUBLICATION_CONFLICT"}
			case errors.Is(e, coordination.ErrDenied):
				response = shared.CodedError{Status: http.StatusForbidden, Code: "EXCHANGE_DENIED"}
			}
			shared.WriteHTTPError(w, response, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "COORDINATION_UNAVAILABLE"})
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
		actor, _ := r.Context().Value(principalKey{}).(principal)
		var item shared.WorkItem
		var created bool
		var createErr error
		e := service.Consume(r.Context(), in.ProjectID, r.PathValue("id"), func(body []byte, key string) (string, error) {
			var spec coordination.WorkRequest
			d := json.NewDecoder(bytes.NewReader(body))
			d.DisallowUnknownFields()
			if d.Decode(&spec) != nil {
				return "", coordination.ErrInvalid
			}
			var extra any
			if d.Decode(&extra) != io.EOF {
				return "", coordination.ErrInvalid
			}
			item, created, createErr = s.createWorkItem(r.Context(), actor, workItemCreate{
				ProjectID: in.ProjectID, Key: key, Instruction: spec.Instruction,
				WorkspaceID: spec.WorkspaceID, Path: spec.Path, Digest: spec.Digest,
				Kind: spec.Kind, ArtifactPaths: spec.ArtifactPaths,
			})
			return item.ID, createErr
		})
		if createErr != nil {
			writeWorkCreateError(w, createErr)
			return
		}
		if e == nil {
			shared.JSON(w, createStatus(created), item)
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
		if e != nil || !s.workspaceInProject(ws, in.ProjectID) {
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
