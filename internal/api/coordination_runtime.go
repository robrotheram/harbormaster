package api

import (
	"agent-workbench/internal/coordination"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type remoteProjectBlobs struct {
	server  *Server
	project string
}

func (b remoteProjectBlobs) Put(ctx context.Context, key string, body []byte) error {
	workspace, _, err := b.server.coordinationBinding(ctx, b.project)
	if err != nil {
		return err
	}
	_, err = b.server.coordinationCall(ctx, b.project, workspace, "POST", "/v1/coordination/blobs/"+key, map[string][]byte{"body": body})
	return err
}
func (b remoteProjectBlobs) Get(ctx context.Context, key string) ([]byte, error) {
	workspace, _, err := b.server.coordinationBinding(ctx, b.project)
	if err != nil {
		return nil, err
	}
	body, err := b.server.coordinationCall(ctx, b.project, workspace, "GET", "/v1/coordination/blobs/"+key, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Body []byte `json:"body"`
	}
	if json.Unmarshal(body, &result) != nil || len(result.Body) > 65536 {
		return nil, errors.New("invalid blob response")
	}
	return result.Body, nil
}
func (s *Server) projectBlobStore(project string) (coordination.BlobStore, error) {
	if !shared.ValidID(project) {
		return nil, coordination.ErrDenied
	}
	return remoteProjectBlobs{s, project}, nil
}
func (s *Server) coordinationBinding(ctx context.Context, project string) (workspace string, notes bool, err error) {
	var enabled int
	err = s.Store.DB.QueryRowContext(ctx, `SELECT workspace_id,notes_enabled FROM project_coordination WHERE project_id=$1`, project).Scan(&workspace, &enabled)
	if err != nil {
		return "", false, coordination.ErrDenied
	}
	return workspace, enabled == 1, nil
}
func (s *Server) coordinationCall(ctx context.Context, project, workspace, method, path string, input any) ([]byte, error) {
	unlock := s.operations.Lock(workspace)
	defer unlock()
	ws, err := s.Store.Workspace(ctx, workspace)
	if err != nil || !s.workspaceInProject(ws, project) {
		return nil, coordination.ErrDenied
	}
	if err = s.lifecycle().ObserveTask(ctx, &ws); err != nil {
		return nil, err
	}
	var body []byte
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			return nil, err
		}
	}
	status, result, err := s.Runtime.Agent(ctx, workspace, method, path, body)
	if err != nil {
		return nil, err
	}
	switch status {
	case 200, 201:
		return result, nil
	case 409:
		return nil, coordination.ErrConflict
	case 400:
		return nil, coordination.ErrInvalid
	case 403, 404:
		return nil, coordination.ErrDenied
	default:
		return nil, fmt.Errorf("coordination guest unavailable: %d", status)
	}
}

type remoteProjectNotes struct {
	server             *Server
	project, workspace string
}

func (n remoteProjectNotes) Publish(in coordination.NoteWrite) (coordination.NoteReceipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var receipt coordination.NoteReceipt
	workspace, enabled, err := n.server.coordinationBinding(ctx, n.project)
	if err != nil || workspace != n.workspace || !enabled {
		return receipt, coordination.ErrDenied
	}
	body, err := n.server.coordinationCall(ctx, n.project, workspace, "POST", "/v1/coordination/notes", in)
	if err != nil {
		return receipt, err
	}
	err = json.Unmarshal(body, &receipt)
	return receipt, err
}
func (s *Server) registerCoordinationRuntime(mux *http.ServeMux) {
	s.RegisterCoordination(mux, s.coordination, func(project, workspace string) (coordination.NotePublisher, error) {
		return remoteProjectNotes{s, project, workspace}, nil
	})
	mux.HandleFunc("POST /v1/projects/{id}/coordination", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) || !s.AuthorizeProject(w, r, r.PathValue("id")) {
			return
		}
		var in struct {
			WorkspaceID  string `json:"workspace_id"`
			NotesEnabled bool   `json:"notes_enabled"`
		}
		if !decodeRouteBody(w, r, &in) {
			return
		}
		workspace, err := s.Store.Workspace(r.Context(), in.WorkspaceID)
		if err != nil || !s.workspaceInProject(workspace, r.PathValue("id")) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		enabled := 0
		if in.NotesEnabled {
			enabled = 1
		}
		result, err := s.Store.DB.ExecContext(r.Context(), `INSERT INTO project_coordination(project_id,workspace_id,notes_enabled) VALUES($1,$2,$3) ON CONFLICT(project_id) DO UPDATE SET notes_enabled=$3 WHERE project_coordination.workspace_id=$2`, r.PathValue("id"), workspace.ID, enabled)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		rows, err := result.RowsAffected()
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		if rows != 1 {
			shared.Error(w, 409, "COORDINATION_BINDING_IMMUTABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"project_id": r.PathValue("id"), "workspace_id": workspace.ID, "notes_enabled": in.NotesEnabled})
	})
}
