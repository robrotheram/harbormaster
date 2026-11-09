package workspace

import (
	"agent-workbench/internal/shared"
	"bytes"
	"encoding/json"
	"sync"
	"time"
)

func (m *Manager) event(source, kind, task, tool, status string) {
	e := shared.ActivityEvent{ID: shared.NewID("evt_"), At: time.Now().UTC(), Source: source, Kind: kind, TaskID: task, Tool: tool, Status: status}
	if !shared.SafeActivityEvent(e) {
		return
	}
	m.eventMu.Lock()
	defer m.eventMu.Unlock()
	now := time.Now()
	if now.Sub(m.eventWindow) > time.Minute {
		m.eventWindow = now
		m.eventCount = 0
	}
	if m.eventCount >= 240 {
		return
	}
	m.eventCount++
	b, _ := json.Marshal(e)
	_, _ = m.db.Exec(`INSERT INTO activity_events(id,at,body) VALUES(?,?,?)`, e.ID, e.At.Format(time.RFC3339Nano), string(b))
	_, _ = m.db.Exec(`DELETE FROM activity_events WHERE id NOT IN (SELECT id FROM activity_events ORDER BY at DESC LIMIT 2000)`)
}
func (m *Manager) events() []shared.ActivityEvent {
	rows, err := m.db.Query(`SELECT body FROM activity_events ORDER BY at DESC LIMIT 200`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []shared.ActivityEvent{}
	for rows.Next() {
		var b string
		var e shared.ActivityEvent
		if rows.Scan(&b) == nil && json.Unmarshal([]byte(b), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

// nativeEvents parses only structured event types. Free text, arguments, output,
// reasoning and error bodies are discarded, including oversized records.
type nativeEvents struct {
	mu           sync.Mutex
	m            *Manager
	source, task string
	buf          []byte
	dropping     bool
}

func (w *nativeEvents) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	for _, b := range p {
		if b == '\n' {
			if !w.dropping {
				w.line(w.buf)
			}
			w.buf = nil
			w.dropping = false
			continue
		}
		if !w.dropping {
			if len(w.buf) >= 1048576 {
				w.buf = nil
				w.dropping = true
			} else {
				w.buf = append(w.buf, b)
			}
		}
	}
	return n, nil
}
func (w *nativeEvents) line(b []byte) {
	w.m.captureHistoryLine(w.source, w.task, b)
	var v struct {
		SchemaVersion int    `json:"schema_version"`
		Kind          string `json:"kind"`
		Tool          string `json:"tool"`
		Type          string `json:"type"`
		Item          struct {
			Type string `json:"type"`
		} `json:"item"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Name string `json:"name"`
			} `json:"content"`
		} `json:"message"`
	}
	if bytes.HasPrefix(b, []byte("AWB_EVENT ")) {
		if json.Unmarshal(b[len("AWB_EVENT "):], &v) != nil || (v.SchemaVersion != 0 && v.SchemaVersion != 1) {
			return
		}
		switch v.Kind {
		case "tool.started", "tool.completed":
			w.m.event(w.source, v.Kind, w.task, shared.ActivityTool(v.Tool), "")
		case "service.connected", "message.received", "response.started", "response.completed", "response.failed":
			if w.source == "matrix" {
				w.m.event(w.source, v.Kind, "", "", "")
			}
		}
		return
	}
	if json.Unmarshal(b, &v) != nil {
		return
	}
	if w.source == "codex" && (v.Type == "item.started" || v.Type == "item.completed") {
		switch v.Item.Type {
		case "command_execution", "file_change", "mcp_tool_call", "web_search":
			kind := "tool.started"
			if v.Type == "item.completed" {
				kind = "tool.completed"
			}
			w.m.event(w.source, kind, w.task, shared.ActivityTool(v.Item.Type), "")
		}
	}
	if w.source == "claude" && v.Type == "assistant" {
		for _, c := range v.Message.Content {
			if c.Type == "tool_use" {
				w.m.event(w.source, "tool.started", w.task, shared.ActivityTool(c.Name), "")
			}
		}
	}
}
