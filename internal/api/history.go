package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
)

var historyUnavailable = shared.CodedError{Status: http.StatusConflict, Code: "HISTORY_UNAVAILABLE"}

func (s *Server) readHistory(ctx context.Context, ws shared.Workspace, q url.Values, filter store.HistoryFilter) (shared.HistoryPage, *store.HistoryCacheState, error) {
	failure := historyUnavailable
	if ws.State == "STOPPED" {
		page, state, err := s.Store.CachedHistory(ctx, ws.ID, filter)
		if err == nil {
			return page, &state, nil
		}
	}
	liveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.refresh(liveCtx, &ws); err == nil {
		if ws.Ready {
			if page, err := s.workerHistory(liveCtx, ws.ID, q); err == nil {
				return page, nil, nil
			}
		} else {
			failure = shared.CodedError{Status: http.StatusConflict, Code: "HISTORY_WORKSPACE_STOPPED"}
		}
	}
	page, state, err := s.Store.CachedHistory(ctx, ws.ID, filter)
	if errors.Is(err, sql.ErrNoRows) {
		return page, nil, failure
	}
	if err != nil {
		return page, nil, storageUnavailable
	}
	return page, &state, nil
}

func (s *Server) workerHistory(ctx context.Context, workspace string, q url.Values) (shared.HistoryPage, error) {
	var page shared.HistoryPage
	q.Set("history", "1")
	q.Set("limit", "20")
	code, b, err := s.Runtime.Agent(ctx, workspace, "GET", "/v1/tasks?"+q.Encode(), nil)
	if err != nil || code != 200 || json.Unmarshal(b, &page) != nil || page.SchemaVersion != 1 {
		return page, historyUnavailable
	}
	if len(page.Records) > 20 || page.HighWatermark < 0 || page.NextAfter < 0 || page.NextBefore < 0 {
		return page, historyUnavailable
	}
	// Ignore purpose claims from the guest. Only operator annotations may label runs.
	for i := range page.Records {
		page.Records[i].Purpose = ""
	}
	return page, nil
}
func (s *Server) serveHistoryPage(w http.ResponseWriter, r *http.Request, ws shared.Workspace, project, episode string) {
	q := r.URL.Query()
	if episode != "" {
		q.Set("episode", episode)
	}
	filter, valid := historyFilter(q)
	if !valid {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	for k := range q {
		switch k {
		case "episode", "after", "before", "until", "order":
		default:
			q.Del(k)
		}
	}
	var page shared.HistoryPage
	var cache *store.HistoryCacheState
	var err error
	if ws.State == "DELETED" {
		var state store.HistoryCacheState
		page, state, err = s.Store.CachedHistory(r.Context(), ws.ID, filter)
		cache = &state
		if errors.Is(err, sql.ErrNoRows) {
			err = historyUnavailable
		}
	} else {
		page, cache, err = s.readHistory(r.Context(), ws, q, filter)
	}
	if err != nil {
		shared.WriteHTTPError(w, err, historyUnavailable)
		return
	}
	ratings, err := s.Store.HistoryFeedback(r.Context(), ws.ID)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	purposes, err := s.Store.HistoryPurposes(r.Context(), ws.ID)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	for i := range page.Records {
		page.Records[i].Purpose = purposes[page.Records[i].EpisodeID]
	}
	if episode != "" {
		for _, record := range page.Records {
			if record.EpisodeID != episode {
				shared.Error(w, 503, "HISTORY_UNAVAILABLE")
				return
			}
		}
		rating, hasRating := ratings[episode]
		clear(ratings)
		if hasRating {
			ratings[episode] = rating
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	shared.JSON(w, 200, map[string]any{"page": page, "cache": cache, "ratings": ratings, "workspace_id": ws.ID, "project_id": project})
}

func (s *Server) registerHistory(mux *http.ServeMux) {
	s.registerProjectAudit(mux)
	mux.HandleFunc("GET /v1/workspaces/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		s.serveHistoryPage(w, r, ws, ws.ProjectID, "")
	})
	mux.HandleFunc("POST /v1/workspaces/{id}/history/purpose", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		var in struct {
			Episode string `json:"episode_id"`
			Purpose string `json:"purpose"`
		}
		if !decodeRouteBody(w, r, &in) {
			return
		}
		if !shared.ValidID(in.Episode) || (in.Purpose != "test" && in.Purpose != "normal") {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		page, _, e := s.readHistory(r.Context(), ws, url.Values{"episode": {in.Episode}}, store.HistoryFilter{Episode: in.Episode})
		if e != nil {
			shared.WriteHTTPError(w, e, historyUnavailable)
			return
		}
		exists := false
		for _, v := range page.Records {
			exists = exists || v.EpisodeID == in.Episode
		}
		if !exists {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if s.Store.SaveHistoryPurpose(r.Context(), ws.ID, in.Episode, in.Purpose) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]string{"episode_id": in.Episode, "purpose": in.Purpose})
	})
	mux.HandleFunc("POST /v1/workspaces/{id}/history/feedback", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		var in struct {
			Episode string `json:"episode_id"`
			Rating  *int   `json:"rating"`
		}
		if !decodeRouteBody(w, r, &in) {
			return
		}
		if !shared.ValidID(in.Episode) || in.Rating == nil || *in.Rating < -1 || *in.Rating > 1 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		page, _, e := s.readHistory(r.Context(), ws, url.Values{"episode": {in.Episode}}, store.HistoryFilter{Episode: in.Episode})
		if e != nil {
			shared.WriteHTTPError(w, e, historyUnavailable)
			return
		}
		if len(page.Records) == 0 {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		v, e := s.Store.SaveHistoryFeedback(r.Context(), ws.ID, in.Episode, *in.Rating)
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, v)
	})
	mux.HandleFunc("GET /v1/workspaces/{id}/history/export", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		page, cache, e := s.readHistory(r.Context(), ws, url.Values{}, store.HistoryFilter{})
		if e != nil {
			shared.WriteHTTPError(w, e, historyUnavailable)
			return
		}
		ratings, e := s.Store.HistoryFeedback(r.Context(), ws.ID)
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		purposes, e := s.Store.HistoryPurposes(r.Context(), ws.ID)
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", `attachment; filename="`+ws.ID+`-history.jsonl"`)
		w.Header().Set("Cache-Control", "no-store")
		enc := json.NewEncoder(w)
		until := page.HighWatermark
		after := int64(0)
		count := 0
		digest := sha256.New()
		if enc.Encode(map[string]any{"type": "manifest", "schema_version": 1, "export_id": shared.NewID("export_"), "workspace_id": ws.ID, "project_id": ws.ProjectID, "exported_at": time.Now().UTC(), "high_watermark": until, "capture_limited": page.CaptureLimited, "cache": cache, "bytes_used": page.BytesUsed, "private_reasoning_excluded": true, "redaction": "best_effort_known_credentials", "feedback_source": "local_operator_only", "purpose_source": "operator_annotation", "agent_kind": ws.AgentKind, "runtime_type": ws.RuntimeType, "branch": ws.Branch}) != nil {
			return
		}
		for {
			for _, v := range page.Records {
				v.Purpose = purposes[v.EpisodeID]
				if v.Seq <= after || v.Seq > until {
					enc.Encode(map[string]any{"type": "export_error", "code": "HISTORY_CURSOR_INVALID"})
					return
				}
				item := map[string]any{"type": "record", "schema_version": 1, "workspace_id": ws.ID, "project_id": ws.ProjectID, "record": v}
				if rating, ok := ratings[v.EpisodeID]; ok {
					item["operator_feedback"] = rating
				}
				b, _ := json.Marshal(item)
				b = append(b, '\n')
				if _, e = w.Write(b); e != nil {
					return
				}
				digest.Write(b)
				after = v.Seq
				count++
			}
			if page.NextAfter == 0 {
				break
			}
			if page.NextAfter != after {
				enc.Encode(map[string]any{"type": "export_error", "code": "HISTORY_CURSOR_INVALID"})
				return
			}
			if cache != nil {
				page, _, e = s.Store.CachedHistory(r.Context(), ws.ID, store.HistoryFilter{After: after, Until: until})
			} else {
				page, e = s.workerHistory(r.Context(), ws.ID, url.Values{"after": {strconv.FormatInt(after, 10)}, "until": {strconv.FormatInt(until, 10)}})
			}
			if e != nil {
				enc.Encode(map[string]any{"type": "export_error", "code": "HISTORY_UNAVAILABLE"})
				return
			}
		}
		if after != until {
			enc.Encode(map[string]any{"type": "export_error", "code": "HISTORY_CURSOR_INVALID"})
			return
		}
		enc.Encode(map[string]any{"type": "export_complete", "records": count, "high_watermark": until, "sha256": hex.EncodeToString(digest.Sum(nil))})
	})
}
