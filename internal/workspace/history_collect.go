package workspace

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Native T3 IDs are namespaced (thread:project:..., item:...), unlike
// workbench IDs. They are only used in an escaped, fixed-origin native URL.
var validT3HistoryID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,511}$`)

func (m *Manager) collectHistory() {
	defer m.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		m.historyCollectMu.Lock()
		m.mu.Lock()
		paused := m.closed || m.quiescing
		m.mu.Unlock()
		if !paused {
			m.backfillTaskHistory()
			if m.console != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				m.collectT3History(ctx)
				cancel()
			}
		}
		m.historyCollectMu.Unlock()
		select {
		case <-m.historyStop:
			return
		case <-ticker.C:
		}
	}
}
func (m *Manager) backfillTaskHistory() {
	rows, e := m.db.Query(`SELECT id,instruction,result,messages,state,created_at,updated_at FROM tasks WHERE state IN ('succeeded','failed','cancelled') AND NOT EXISTS(SELECT 1 FROM agent_history WHERE episode_id=tasks.id) ORDER BY created_at LIMIT 10`)
	if e != nil {
		return
	}
	type item struct{ id, prompt, result, messages, state, created, updated string }
	items := []item{}
	for rows.Next() {
		var v item
		if rows.Scan(&v.id, &v.prompt, &v.result, &v.messages, &v.state, &v.created, &v.updated) == nil {
			items = append(items, v)
		}
	}
	rows.Close()
	for _, v := range items {
		created, _ := time.Parse(time.RFC3339Nano, v.created)
		updated, _ := time.Parse(time.RFC3339Nano, v.updated)
		m.history(shared.HistoryRecord{ID: stableHistoryID(v.id, "user.message", "prompt"), EpisodeID: v.id, Source: "workspace", Kind: "user.message", Role: "user", Content: v.prompt, At: created, CaptureScope: "backfill_task", Backfilled: true})
		var messages []Message
		if json.Unmarshal([]byte(v.messages), &messages) == nil {
			for _, msg := range messages {
				for _, call := range msg.ToolCalls {
					m.history(shared.HistoryRecord{ID: stableHistoryID(v.id, "tool.call", call.ID), EpisodeID: v.id, Source: "workspace", Kind: "tool.call", Role: "assistant", Tool: call.Function.Name, ToolCallID: call.ID, Content: call.Function.Arguments, At: updated, Backfilled: true, CaptureScope: "backfill_task"})
				}
				if msg.Role == "tool" {
					m.history(shared.HistoryRecord{ID: stableHistoryID(v.id, "tool.result", msg.ToolCallID), EpisodeID: v.id, Source: "workspace", Kind: "tool.result", Role: "tool", ToolCallID: msg.ToolCallID, Content: msg.Content, At: updated, Backfilled: true, CaptureScope: "backfill_task"})
				}
			}
		}
		if v.result != "" {
			m.history(shared.HistoryRecord{ID: stableHistoryID(v.id, "assistant.message", "final"), EpisodeID: v.id, Source: "workspace", Kind: "assistant.message", Role: "assistant", Content: v.result, At: updated, Backfilled: true, CaptureScope: "backfill_task"})
		}
		kind := "run.completed"
		if v.state == "failed" {
			kind = "run.failed"
		}
		if v.state == "cancelled" {
			kind = "run.cancelled"
		}
		m.history(shared.HistoryRecord{EpisodeID: v.id, Source: "workspace", Kind: kind, Status: v.state, At: updated, Backfilled: true, CaptureScope: "backfill_task"})
	}
}
func (m *Manager) t3HistoryJSON(ctx context.Context, path string, out any) bool {
	token, e := m.console.session(ctx)
	if e != nil {
		return false
	}
	r, e := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:3773"+path, nil)
	if e != nil {
		return false
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("x-t3-orchestration-protocol", "2")
	res, e := consoleHTTP.Do(r)
	if e != nil {
		return false
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	return e == nil && len(b) <= 8<<20 && res.StatusCode == 200 && json.Unmarshal(b, out) == nil
}
func (m *Manager) collectT3History(ctx context.Context) {
	var shell struct {
		Projects []struct {
			ID   string `json:"id"`
			Root string `json:"workspaceRoot"`
		} `json:"projects"`
		Threads  []t3HistoryThread `json:"threads"`
		Archived []t3HistoryThread `json:"archivedThreads"`
	}
	if !m.t3HistoryJSON(ctx, "/api/orchestration/shell", &shell) {
		return
	}
	allowed := map[string]bool{}
	for _, p := range shell.Projects {
		if p.Root == filepath.Join(m.cfg.DataDir, "files") {
			allowed[p.ID] = true
		}
	}
	if m.historyT3Seen == nil {
		m.historyT3Seen = map[string]string{}
	}
	processed := 0
	nextOffset := m.historyT3Offset
	defer func() { m.historyT3Offset = nextOffset }()
	threads := append(shell.Threads, shell.Archived...)
	for offset := 0; offset < len(threads); offset++ {
		thread := threads[(m.historyT3Offset+offset)%len(threads)]
		if !allowed[thread.Project] || !validT3HistoryID.MatchString(thread.ID) || m.historyT3Seen[thread.ID] == thread.Updated {
			continue
		}
		if processed >= 3 || ctx.Err() != nil {
			break
		}
		processed++
		nextOffset = (m.historyT3Offset + offset + 1) % len(threads)
		var transcript t3HistoryTranscript
		if !m.t3HistoryJSON(ctx, "/api/orchestration/threads/"+url.PathEscape(thread.ID)+"/transcript", &transcript) {
			episode := stableHistoryID(thread.ID, "gap", "transcript")
			m.history(shared.HistoryRecord{ID: episode, EpisodeID: episode, SessionID: thread.ID, Source: "t3", Kind: "capture.gap", Content: "Native transcript unavailable or exceeds 8 MiB. This session's exported history may be incomplete.", CaptureScope: "t3_projection"})
			continue
		}
		m.appendT3History(thread, transcript)
		m.historyT3Seen[thread.ID] = thread.Updated
	}
}

type t3HistoryThread struct {
	ID        string `json:"id"`
	Project   string `json:"projectId"`
	Updated   string `json:"updatedAt"`
	Provider  string `json:"providerInstanceId"`
	Selection struct {
		Model string `json:"model"`
	} `json:"modelSelection"`
}
type t3HistoryTranscript struct {
	Items []struct {
		Visibility string          `json:"visibility"`
		Item       json.RawMessage `json:"item"`
	} `json:"items"`
}

func (m *Manager) appendT3History(thread t3HistoryThread, transcript t3HistoryTranscript) {
	for _, row := range transcript.Items {
		if row.Visibility != "local" {
			continue
		}
		var v struct {
			ID            string          `json:"id"`
			RunID         string          `json:"runId"`
			Type          string          `json:"type"`
			Text          string          `json:"text"`
			Input         json.RawMessage `json:"input"`
			Output        json.RawMessage `json:"output"`
			Tool          string          `json:"toolName"`
			File          string          `json:"fileName"`
			Diff          string          `json:"diffStr"`
			Updated       string          `json:"updatedAt"`
			Status        string          `json:"status"`
			Streaming     bool            `json:"streaming"`
			TextOmitted   bool            `json:"textOmitted"`
			OutputOmitted bool            `json:"outputOmitted"`
		}
		if json.Unmarshal(row.Item, &v) != nil || v.Streaming || !validT3HistoryID.MatchString(v.ID) {
			continue
		}
		episode := stableHistoryID(thread.ID, "run", v.RunID)
		at, _ := time.Parse(time.RFC3339Nano, v.Updated)
		base := shared.HistoryRecord{EpisodeID: episode, SessionID: thread.ID, NativeRunID: v.RunID, Source: "t3", Model: thread.Selection.Model, Provider: thread.Provider, At: at, CaptureScope: "t3_projection", Backfilled: true, Status: v.Status}
		appendRecord := func(kind, role, content, tool string, truncated bool) {
			r := base
			r.ID = stableHistoryID(thread.ID, kind, v.ID)
			r.Kind = kind
			r.Role = role
			r.Content = content
			r.Tool = tool
			if kind == "user.message" || kind == "assistant.message" {
				r.MessageID = v.ID
			} else {
				r.ToolCallID = v.ID
			}
			r.Truncated = truncated
			m.history(r)
		}
		switch v.Type {
		case "user_message":
			appendRecord("user.message", "user", v.Text, "", v.TextOmitted)
		case "assistant_message":
			appendRecord("assistant.message", "assistant", v.Text, "", v.TextOmitted)
		case "command_execution", "dynamic_tool":
			tool := v.Tool
			if v.Type == "command_execution" {
				tool = "terminal"
			}
			input := string(v.Input)
			if strings.HasPrefix(input, "\"") {
				json.Unmarshal(v.Input, &input)
			}
			appendRecord("tool.call", "assistant", input, tool, false)
			if v.Status != "running" && v.Status != "pending" {
				output := string(v.Output)
				if strings.HasPrefix(output, "\"") {
					json.Unmarshal(v.Output, &output)
				}
				appendRecord("tool.result", "tool", output, tool, v.OutputOmitted)
			}
		case "file_change":
			appendRecord("tool.result", "tool", v.File+"\n"+v.Diff, "file_change", v.Diff == "")
			// reasoning, system notices, secret requests, provider payloads and
			// compaction summaries are deliberately not imported.
		}
	}
}
