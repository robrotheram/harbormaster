package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

func (s *Server) workHistory(repo orchestration.Repository, live bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" {
			shared.Error(w, 400, "PROJECT_REQUIRED")
			return
		}
		ctx := r.Context()
		attempt, err := repo.Attempt(ctx, project, r.PathValue("id"), r.PathValue("attempt"))
		if err != nil {
			workError(w, err)
			return
		}
		if !shared.ValidID(attempt.TaskID) || !shared.ValidID(attempt.WorkspaceID) {
			shared.Error(w, 409, "HISTORY_UNAVAILABLE")
			return
		}
		ws, err := s.Store.Workspace(ctx, attempt.WorkspaceID)
		if errors.Is(err, sql.ErrNoRows) {
			// Attempt provenance survives workspace deletion.
			ws = shared.Workspace{ID: attempt.WorkspaceID, State: "DELETED"}
		} else if err != nil || !s.workspaceInProject(ws, project) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if live {
			if ws.State == "DELETED" || ws.State == "STOPPED" {
				shared.WriteHTTPError(w, historyUnavailable, historyUnavailable)
				return
			}
			s.streamWorkHistory(w, r, repo, project, attempt)
			return
		}
		s.serveHistoryPage(w, r, ws, project, attempt.TaskID)
	}
}

func (s *Server) streamWorkHistory(w http.ResponseWriter, r *http.Request, repo orchestration.Repository, project string, attempt shared.WorkAttempt) {
	query := r.URL.Query()
	for key, values := range query {
		if (key != "after" && key != "project_id") || len(values) != 1 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
	}
	filter, valid := historyFilter(query)
	if last := r.Header.Get("Last-Event-ID"); valid && last != "" {
		after, err := strconv.ParseInt(last, 10, 64)
		valid = err == nil && after >= 0
		filter.After = max(filter.After, after)
	}
	if !valid {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}

	// Reconnect periodically through the dashboard to renew its session check.
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "private, no-store, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	response := http.NewResponseController(w)
	defer response.SetWriteDeadline(time.Time{})
	flush := func(frame string) bool {
		_ = response.SetWriteDeadline(time.Now().Add(15 * time.Second))
		_, err := fmt.Fprint(w, frame)
		return err == nil && response.Flush() == nil
	}
	if !flush("retry: 1000\n\n") {
		return
	}
	cursor := filter.After
	p, _ := ctx.Value(principalKey{}).(principal)
	for ctx.Err() == nil {
		current, err := repo.Attempt(ctx, project, attempt.WorkItemID, attempt.ID)
		if err != nil || current.TaskID != attempt.TaskID || current.WorkspaceID != attempt.WorkspaceID {
			flush("event: unavailable\ndata: {}\n\n")
			return
		}
		workerQuery := url.Values{"episode": {attempt.TaskID}, "after": {strconv.FormatInt(cursor, 10)}}
		active := shared.Active(current.State)
		if active {
			workerQuery.Set("wait", "1")
		}
		started := time.Now()
		pageCtx, stop := context.WithTimeout(ctx, shared.HistoryWait+3*time.Second)
		page, err := s.workerHistory(pageCtx, attempt.WorkspaceID, workerQuery)
		stop()
		if ctx.Err() != nil {
			return
		}
		if err != nil || s.authorizeProject(ctx, p, project) != nil || !validWorkHistoryPage(page, attempt.TaskID, cursor) {
			flush("event: unavailable\ndata: {}\n\n")
			return
		}
		if len(page.Records) > 0 || page.CaptureLimited {
			if len(page.Records) > 0 {
				cursor = page.Records[len(page.Records)-1].Seq
			}
			data, _ := json.Marshal(map[string]any{"page": page, "workspace_id": attempt.WorkspaceID, "project_id": project})
			if !flush(fmt.Sprintf("id: %d\nevent: history\ndata: %s\n\n", cursor, data)) {
				return
			}
		}
		if !active && page.NextAfter == 0 {
			flush("event: done\ndata: {}\n\n")
			return
		}
		if len(page.Records) == 0 {
			// Older workers ignore the wait parameter. Never spin on an empty page.
			timer := time.NewTimer(max(0, time.Second-time.Since(started)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if !flush(": keepalive\n\n") {
				return
			}
		}
	}
}

func validWorkHistoryPage(page shared.HistoryPage, episode string, after int64) bool {
	last := after
	for _, record := range page.Records {
		if record.EpisodeID != episode || record.Seq <= last || record.Seq > page.HighWatermark ||
			record.SchemaVersion != 1 || !shared.ValidID(record.ID) || len(record.Content) > 32<<10 {
			return false
		}
		last = record.Seq
	}
	return page.NextAfter == 0 || (last > after && page.NextAfter == last)
}
