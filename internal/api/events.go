package api

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

// Only safe metadata is
// mirrored into platform storage, so logs remain readable after a VM stops.
func (s *Server) captureEvents(ctx context.Context, ws shared.Workspace, body []byte) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()
	if s.eventSeen == nil {
		s.eventSeen = map[string]map[string]bool{}
	}
	seen := s.eventSeen[ws.ID]
	if seen == nil || len(seen) > 2400 {
		seen = map[string]bool{}
		s.eventSeen[ws.ID] = seen
	}
	var list struct {
		Events    []shared.ActivityEvent `json:"events"`
		Supported bool                   `json:"events_supported"`
	}
	batch := []shared.ActivityEvent{}
	if json.Unmarshal(body, &list) == nil {
		for i, v := range list.Events {
			if i >= 200 {
				break
			}
			if shared.SafeActivityEvent(v) && !seen[v.ID] {
				batch = append(batch, v)
			}
		}
	}
	if s.eventState == nil {
		s.eventState = map[string]string{}
	}
	state := fmt.Sprintf("%s/%d/%d", ws.State, ws.ActiveRuns, ws.BackgroundJobs)
	if s.eventState[ws.ID] != state {
		batch = append(batch, shared.ActivityEvent{ID: shared.NewID("evt_"), At: s.now(), Source: "workspace", Kind: "workspace.state", Status: ws.State})
	}
	// Native task metadata provides baseline history for older guest images.
	var tasks struct {
		Items []shared.Task `json:"items"`
	}
	if !list.Supported && json.Unmarshal(body, &tasks) == nil {
		for i, t := range tasks.Items {
			if i >= 1000 {
				break
			}
			if !validTask(t, ws.ID) {
				continue
			}
			hash := sha256.Sum256([]byte(ws.ID + "/" + t.ID + "/" + t.State + "/" + t.UpdatedAt.String()))
			eventID := "evt_" + hex.EncodeToString(hash[:16])
			if seen[eventID] {
				continue
			}
			kind := "task." + t.State
			switch t.State {
			case "running":
				kind = "task.started"
			case "queued", "succeeded", "failed", "cancelled":
			default:
				continue
			}
			batch = append(batch, shared.ActivityEvent{ID: eventID, At: t.UpdatedAt, Source: "workspace", Kind: kind, TaskID: t.ID})
		}
	}
	if len(batch) == 0 {
		return
	}
	if s.Store.SaveEvents(ctx, ws, batch) == nil {
		for _, v := range batch {
			seen[v.ID] = true
		}
		s.eventState[ws.ID] = state
	}
}
func (s *Server) registerEvents(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/workspaces/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		events, e := s.Store.Events(r.Context(), ws.ProjectID, ws.ID)
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		services := []map[string]string{}
		if ws.Ready && ws.AgentKind == "hermes" {
			status, b, e := s.Runtime.Agent(r.Context(), ws.ID, "GET", "/v1/matrix/status", nil)
			var state struct {
				State string `json:"state"`
			}
			if e == nil && status == 200 && json.Unmarshal(b, &state) == nil {
				switch state.State {
				case "connected", "starting", "failed", "disconnected":
					services = append(services, map[string]string{"name": "Matrix listener", "state": state.State})
				}
			}
		}
		shared.JSON(w, 200, map[string]any{"items": events, "services": services, "workspace_id": ws.ID, "project_id": ws.ProjectID, "state": ws.State, "active_runs": ws.ActiveRuns, "background_jobs": ws.BackgroundJobs})
	})
	mux.HandleFunc("GET /v1/workspace-logs", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		events, e := s.Store.Events(r.Context(), project, "")
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": events})
	})
}
