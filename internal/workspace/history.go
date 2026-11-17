package workspace

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const historyBudget int64 = 256 << 20

var historyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)Bearer\s+[a-z0-9_./+\-=]+`),
	regexp.MustCompile(`(?i)\b(?:sk-[a-z0-9_-]{12,}|mct_[a-z0-9_-]+|syt_[a-z0-9_-]+|gh[pousr]_[a-z0-9_]+|github_pat_[a-z0-9_]+)`),
	regexp.MustCompile(`(?i)(?:"?(?:access_token|refresh_token|api_key|password|client_secret|authorization|vault_token|workbench_inference_grant)"?\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;}]+)`),
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
}

func (m *Manager) redactHistorySecrets(text string) (string, bool) {
	original := text
	if m.cfg.Token != "" {
		text = strings.ReplaceAll(text, m.cfg.Token, "[REDACTED]")
	}
	for _, p := range historyPatterns {
		text = p.ReplaceAllString(text, "[REDACTED]")
	}
	return text, text != original
}
func trimHistoryContent(text string) (string, bool) {
	truncated := len(text) > 32768
	if truncated {
		n := 32768
		for n > 0 && !utf8.RuneStart(text[n]) {
			n--
		}
		text = text[:n]
	}
	return strings.ToValidUTF8(text, "�"), truncated
}
func (m *Manager) redactHistory(text string) (string, bool, bool) {
	text, redacted := m.redactHistorySecrets(text)
	text, truncated := trimHistoryContent(text)
	return text, redacted, truncated
}

// Preserve structured tool payloads as valid JSON when replacing credentials.
// Oversized payloads are explicitly flagged as truncated, regardless of format.
func (m *Manager) redactHistoryContent(text string) (string, bool, bool) {
	var value any
	if (strings.HasPrefix(strings.TrimSpace(text), "{") || strings.HasPrefix(strings.TrimSpace(text), "[")) && json.Unmarshal([]byte(text), &value) == nil {
		changed := false
		var walk func(any) any
		walk = func(v any) any {
			switch x := v.(type) {
			case map[string]any:
				for k, item := range x {
					switch strings.ToLower(k) {
					case "access_token", "refresh_token", "api_key", "password", "client_secret", "authorization", "vault_token", "workbench_inference_grant":
						x[k] = "[REDACTED]"
						changed = true
					default:
						x[k] = walk(item)
					}
				}
			case []any:
				for i := range x {
					x[i] = walk(x[i])
				}
			case string:
				cleaned, redacted := m.redactHistorySecrets(x)
				changed = changed || redacted
				return cleaned
			}
			return v
		}
		value = walk(value)
		if changed {
			b, _ := json.Marshal(value)
			text = string(b)
		}
		content, truncated := trimHistoryContent(text)
		return content, changed, truncated
	}
	return m.redactHistory(text)
}
func (m *Manager) history(v shared.HistoryRecord) {
	switch v.Kind {
	case "user.message", "assistant.message", "tool.call", "tool.result", "run.started", "run.completed", "run.failed", "run.cancelled", "run.usage", "capture.gap", "message.delivery":
	default:
		return
	}
	switch v.Source {
	case "workspace", "codex", "claude", "hermes", "matrix", "t3":
	default:
		return
	}
	if !shared.ValidID(v.EpisodeID) {
		return
	}
	// Do not collect thinking tools or hidden reasoning channels.
	lower := strings.ToLower(v.Tool)
	if strings.Contains(lower, "think") || strings.Contains(lower, "reason") || strings.Contains(lower, "reflection") {
		return
	}
	if len(v.Tool) > 128 || len(v.ToolCallID) > 512 || len(v.SessionID) > 512 || len(v.RoomID) > 255 || len(v.SenderID) > 255 || len(v.MessageID) > 512 || len(v.RequesterID) > 255 || len(v.NativeRunID) > 512 {
		return
	}
	content, redacted, truncated := m.redactHistoryContent(v.Content)
	v.Content = content
	v.Redacted = v.Redacted || redacted
	v.Truncated = v.Truncated || truncated
	if v.At.IsZero() {
		v.At = time.Now().UTC()
	}
	v.SchemaVersion = 1
	v.CapturedAt = time.Now().UTC()
	if v.CaptureScope == "" {
		v.CaptureScope = "live"
	}
	if v.ID == "" {
		v.ID = shared.NewID("hist_")
	}
	if !shared.ValidID(v.ID) {
		return
	}
	v.Seq = 0
	if v.Model == "" && !v.Backfilled {
		switch v.Source {
		case "hermes", "matrix":
			v.Model = m.cfg.HermesModel
			v.Provider = "openai-codex"
			v.EngineVersion = "v0.23.0"
		case "codex":
			v.Model = m.cfg.CodexModel
			v.Provider = "openai-codex"
		case "claude":
			v.Model = m.cfg.ClaudeModel
			v.Provider = "anthropic"
		case "workspace":
			v.Model = m.cfg.Model
			v.Synthetic = m.cfg.Model == "fixture"
		}
	}
	if !v.Backfilled && v.EpisodeID != "" {
		var selected string
		if err := m.db.QueryRow(`SELECT model FROM file_acceptances WHERE task_id=?`, v.EpisodeID).Scan(&selected); err == nil && selected != "" {
			v.Model = selected
		}
	}
	// All string fields use the same redaction boundary, including source IDs.
	fields := []*string{&v.Tool, &v.ToolCallID, &v.SessionID, &v.RoomID, &v.SenderID, &v.Model, &v.Provider, &v.Status, &v.MessageID, &v.RequesterID, &v.NativeRunID}
	for _, f := range fields {
		s, r, t := m.redactHistory(*f)
		*f = s
		v.Redacted = v.Redacted || r
		v.Truncated = v.Truncated || t
	}
	b, e := json.Marshal(v)
	if e != nil || len(b) > 65536 {
		return
	}
	tx, e := m.db.Begin()
	if e != nil {
		return
	}
	defer tx.Rollback()
	var exists int
	if tx.QueryRow(`SELECT count(*) FROM agent_history WHERE id=?`, v.ID).Scan(&exists) != nil || exists != 0 {
		return
	}
	var used int64
	if tx.QueryRow(`SELECT bytes_used FROM history_state WHERE id=1`).Scan(&used) != nil {
		return
	}
	if used+int64(len(b)) > historyBudget {
		tx.Exec(`UPDATE history_state SET limited=1 WHERE id=1`)
		tx.Commit()
		return
	}
	result, e := tx.Exec(`INSERT INTO agent_history(id,episode_id,body) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING`, v.ID, v.EpisodeID, string(b))
	if e != nil {
		return
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		tx.Exec(`UPDATE history_state SET bytes_used=bytes_used+? WHERE id=1`, len(b))
	}
	tx.Commit()
}
func (m *Manager) historyPage(w http.ResponseWriter, r *http.Request) {
	values := map[string]int64{"after": 0, "before": 0, "until": 0, "limit": 20}
	for k := range values {
		if s := r.URL.Query().Get(k); s != "" {
			v, e := strconv.ParseInt(s, 10, 64)
			if e != nil || v < 0 {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			values[k] = v
		}
	}
	if values["limit"] < 1 || values["limit"] > 20 {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	out := shared.HistoryPage{Records: []shared.HistoryRecord{}, SchemaVersion: 1}
	var limited int
	if m.db.QueryRow(`SELECT bytes_used,limited FROM history_state WHERE id=1`).Scan(&out.BytesUsed, &limited) != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	out.CaptureLimited = limited != 0
	m.db.QueryRow(`SELECT COALESCE(MAX(seq),0) FROM agent_history`).Scan(&out.HighWatermark)
	until := values["until"]
	if until == 0 {
		until = out.HighWatermark
	}
	q := `SELECT seq,body FROM agent_history WHERE seq>? AND seq<=?`
	args := []any{values["after"], until}
	if values["before"] > 0 {
		q += ` AND seq<?`
		args = append(args, values["before"])
	}
	order := "ASC"
	if r.URL.Query().Get("order") == "desc" {
		order = "DESC"
	}
	if episode := r.URL.Query().Get("episode"); episode != "" {
		if !shared.ValidID(episode) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		q += ` AND episode_id=?`
		args = append(args, episode)
	}
	q += ` ORDER BY seq ` + order + ` LIMIT ?`
	args = append(args, values["limit"])
	rows, e := m.db.Query(q, args...)
	if e != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var seq int64
		var b string
		var v shared.HistoryRecord
		if rows.Scan(&seq, &b) != nil || json.Unmarshal([]byte(b), &v) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		v.Seq = seq
		out.Records = append(out.Records, v)
	}
	if rows.Err() != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	if len(out.Records) == int(values["limit"]) {
		last := out.Records[len(out.Records)-1].Seq
		if order == "ASC" {
			out.NextAfter = last
		} else {
			out.NextBefore = last
		}
	}
	shared.JSON(w, 200, out)
}
func stableHistoryID(episode, kind, key string) string {
	h := sha256.Sum256([]byte(episode + "\x00" + kind + "\x00" + key))
	return "hist_" + hex.EncodeToString(h[:16])
}

// Result history is recorded inside the private workspace database only.
func (m *Manager) historyTaskResult(_ context.Context, id, provider, result, code, state string, elapsed time.Duration) {
	if provider == "" {
		provider = "workspace"
	}
	if result != "" {
		m.history(shared.HistoryRecord{ID: stableHistoryID(id, "assistant.message", "final"), EpisodeID: id, Source: provider, Kind: "assistant.message", Role: "assistant", Content: result})
	}
	kind := "run.completed"
	if state == "failed" {
		kind = "run.failed"
	}
	if state == "cancelled" {
		kind = "run.cancelled"
	}
	m.history(shared.HistoryRecord{EpisodeID: id, Source: provider, Kind: kind, Status: state, ElapsedMS: elapsed.Milliseconds()})
}
