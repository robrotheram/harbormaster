package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
)

func historyFilter(q url.Values) (store.HistoryFilter, bool) {
	filter := store.HistoryFilter{Episode: q.Get("episode"), Descending: q.Get("order") == "desc"}
	if filter.Episode != "" && !shared.ValidID(filter.Episode) {
		return filter, false
	}
	for _, cursor := range []struct {
		key string
		dst *int64
	}{{"after", &filter.After}, {"before", &filter.Before}, {"until", &filter.Until}} {
		if raw := q.Get(cursor.key); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 0 {
				return filter, false
			}
			*cursor.dst = n
		}
	}
	return filter, true
}

// Only callers with a ready observation sync history. This never starts
// compute or advances the cursor past an uncommitted page.
func (s *Server) syncHistory(ctx context.Context, ws shared.Workspace, pages int, budget time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	state, err := s.Store.HistoryCacheState(ctx, ws.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.reportWrite("history_cache_state", ws.ID, err)
		return
	}
	after, until := state.Cursor, int64(0)
	if s.sharedWorkspace(ws) || ws.Shared {
		// The runtime owner is not the owner of all shared conversations.
		ws.ProjectID = ""
	}
	for i := 0; i < pages; i++ {
		q := url.Values{"after": {strconv.FormatInt(after, 10)}}
		if until > 0 {
			q.Set("until", strconv.FormatInt(until, 10))
		}
		page, err := s.workerHistory(ctx, ws.ID, q)
		if err != nil || page.HighWatermark < after || page.BytesUsed < 0 || (until > 0 && page.HighWatermark < until) {
			return
		}
		if until == 0 {
			until = page.HighWatermark
		}
		last := after
		for _, record := range page.Records {
			if record.Seq <= last || record.Seq > until || record.SchemaVersion != 1 ||
				!shared.ValidID(record.ID) || !shared.ValidID(record.EpisodeID) || len(record.Content) > 32<<10 {
				return
			}
			last = record.Seq
		}
		if (page.NextAfter != 0 && (page.NextAfter != last || last == after)) ||
			(page.NextAfter == 0 && last != until) {
			return
		}
		ok, err := s.Store.AppendHistoryCache(ctx, ws, after, page, s.now())
		s.reportWrite("history_cache_append", ws.ID, err)
		if err != nil || !ok || last == until {
			return
		}
		after = last
	}
}

func (s *Server) registerProjectAudit(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/projects/{id}/audit-history", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("id")
		if !s.AuthorizeProject(w, r, project) || (s.cfg.SharedWorkspaceID != "" && !s.admin(w, r)) {
			return
		}
		q := r.URL.Query()
		filter, ok := historyFilter(q)
		worker, kind := q.Get("workspace_id"), q.Get("kind")
		if !ok || (worker != "" && !shared.ValidID(worker)) || len(kind) > 32 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		items, next, err := s.Store.ProjectAudit(r.Context(), project, s.cfg.SharedWorkspaceID, worker, kind, filter.Before)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		shared.JSON(w, 200, map[string]any{"project_id": project, "items": items, "next_before": next})
	})
}
