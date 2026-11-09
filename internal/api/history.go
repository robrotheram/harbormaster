package api

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func (s *Server) readHistory(ctx context.Context, ws shared.Workspace, q url.Values) (shared.HistoryPage, error) {
	var page shared.HistoryPage
	if err := s.refresh(ctx, &ws); err != nil {
		return page, errors.New("HISTORY_UNAVAILABLE")
	}
	if !ws.Ready {
		return page, errors.New("HISTORY_WORKSPACE_STOPPED")
	}
	q.Set("history", "1")
	q.Set("limit", "20")
	code, b, err := s.Runtime.Agent(ctx, ws.ID, "GET", "/v1/tasks?"+q.Encode(), nil)
	if err != nil || code != 200 || json.Unmarshal(b, &page) != nil || page.SchemaVersion != 1 {
		return page, errors.New("HISTORY_UNAVAILABLE")
	}
	if len(page.Records) > 20 || page.HighWatermark < 0 || page.NextAfter < 0 || page.NextBefore < 0 {
		return page, errors.New("HISTORY_UNAVAILABLE")
	}
	// Ignore purpose claims from the guest. Only operator annotations may label runs.
	for i := range page.Records {
		page.Records[i].Purpose = ""
	}
	return page, nil
}
func historyError(w http.ResponseWriter, err error) {
	code := "HISTORY_UNAVAILABLE"
	if err.Error() == "HISTORY_WORKSPACE_STOPPED" {
		code = err.Error()
	}
	shared.Error(w, 409, code)
}
func (s *Server) registerHistory(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/workspaces/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		q := url.Values{}
		for _, k := range []string{"after", "before", "until"} {
			if v := r.URL.Query().Get(k); v != "" {
				n, e := strconv.ParseInt(v, 10, 64)
				if e != nil || n < 0 {
					shared.Error(w, 400, "INVALID_REQUEST")
					return
				}
				q.Set(k, v)
			}
		}
		if r.URL.Query().Get("order") == "desc" {
			q.Set("order", "desc")
		}
		page, e := s.readHistory(r.Context(), ws, q)
		if e != nil {
			historyError(w, e)
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
		for i := range page.Records {
			page.Records[i].Purpose = purposes[page.Records[i].EpisodeID]
		}
		// No message content is stored in the platform database.
		shared.JSON(w, 200, map[string]any{"page": page, "ratings": ratings, "workspace_id": ws.ID, "project_id": ws.ProjectID})
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
		if !shared.Decode(w, r, &in, 2048) {
			return
		}
		if !shared.ValidID(in.Episode) || (in.Purpose != "test" && in.Purpose != "normal") {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		page, e := s.readHistory(r.Context(), ws, url.Values{"episode": []string{in.Episode}})
		if e != nil {
			historyError(w, e)
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
		if !shared.Decode(w, r, &in, 2048) {
			return
		}
		if !shared.ValidID(in.Episode) || in.Rating == nil || *in.Rating < -1 || *in.Rating > 1 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		page, e := s.readHistory(r.Context(), ws, url.Values{"episode": []string{in.Episode}})
		if e != nil {
			historyError(w, e)
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
		page, e := s.readHistory(r.Context(), ws, url.Values{})
		if e != nil {
			historyError(w, e)
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
		if enc.Encode(map[string]any{"type": "manifest", "schema_version": 1, "export_id": shared.NewID("export_"), "workspace_id": ws.ID, "project_id": ws.ProjectID, "exported_at": time.Now().UTC(), "high_watermark": until, "capture_limited": page.CaptureLimited, "bytes_used": page.BytesUsed, "private_reasoning_excluded": true, "redaction": "best_effort_known_credentials", "feedback_source": "local_operator_only", "purpose_source": "operator_annotation", "agent_kind": ws.AgentKind, "runtime_type": ws.RuntimeType, "branch": ws.Branch}) != nil {
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
			page, e = s.readHistory(r.Context(), ws, url.Values{"after": []string{strconv.FormatInt(after, 10)}, "until": []string{strconv.FormatInt(until, 10)}})
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
