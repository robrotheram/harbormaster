package workspace

import (
	"agent-workbench/internal/shared"
	"bytes"
	"encoding/json"
	"strings"
)

func (m *Manager) captureHistoryLine(source, task string, b []byte) {
	if bytes.HasPrefix(b, []byte("AWB_HISTORY ")) {
		var frame struct {
			SchemaVersion int    `json:"schema_version"`
			Episode       string `json:"episode"`
			Kind          string `json:"kind"`
			Tool          string `json:"tool"`
			CallID        string `json:"call_id"`
			Content       string `json:"content"`
			Room          string `json:"room_id"`
			Sender        string `json:"sender_id"`
			MessageID     string `json:"message_id"`
			Status        string `json:"status"`
			InputTokens   int64  `json:"input_tokens"`
			OutputTokens  int64  `json:"output_tokens"`
			ElapsedMS     int64  `json:"elapsed_ms"`
			Truncated     bool   `json:"truncated"`
		}
		if json.Unmarshal(b[len("AWB_HISTORY "):], &frame) != nil || (frame.SchemaVersion != 0 && frame.SchemaVersion != 1) {
			return
		}
		role := ""
		switch frame.Kind {
		case "user.message":
			role = "user"
		case "assistant.message", "tool.call":
			role = "assistant"
		case "tool.result":
			role = "tool"
		case "run.started", "run.completed", "run.failed", "run.usage":
		case "message.delivery":
			role = "assistant"
		default:
			return
		}
		episode := task
		session := ""
		channel := "workspace"
		if source == "matrix" {
			if m.cfg.HermesMatrix == nil {
				return
			}
			room, sender := false, false
			for _, r := range m.cfg.HermesMatrix.RoomIDs {
				room = room || r == frame.Room
			}
			for _, s := range m.cfg.HermesMatrix.SenderIDs {
				sender = sender || s == frame.Sender
			}
			if !room || !sender || !shared.ValidID(frame.Episode) {
				return
			}
			episode = frame.Episode
			session = stableHistoryID(frame.Room, "matrix", frame.Sender)
			channel = "matrix"
		}
		if !shared.ValidID(episode) {
			return
		}
		id := ""
		if frame.Kind == "user.message" || frame.Kind == "assistant.message" {
			id = stableHistoryID(episode, frame.Kind, "final")
		}
		sender := frame.Sender
		if source == "matrix" && role == "assistant" {
			sender = m.cfg.HermesMatrix.BotUserID
		}
		m.history(shared.HistoryRecord{ID: id, RequesterID: frame.Sender, EpisodeID: episode, SessionID: session, Source: source, Kind: frame.Kind, Role: role, Tool: frame.Tool, ToolCallID: frame.CallID, Content: frame.Content, RoomID: frame.Room, SenderID: sender, MessageID: frame.MessageID, Status: frame.Status, Channel: channel, InputTokens: frame.InputTokens, OutputTokens: frame.OutputTokens, UsageScope: "session_cumulative", ElapsedMS: frame.ElapsedMS, Truncated: frame.Truncated})
		return
	}
	if source != "codex" && source != "claude" {
		return
	}
	var frame struct {
		Type string `json:"type"`
		Item struct {
			ID        string          `json:"id"`
			Type      string          `json:"type"`
			Command   string          `json:"command"`
			Output    string          `json:"aggregated_output"`
			Name      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
			Result    json.RawMessage `json:"result"`
			Changes   json.RawMessage `json:"changes"`
			Status    string          `json:"status"`
		} `json:"item"`
		Usage struct {
			Input  int64 `json:"input_tokens"`
			Output int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(b, &frame) != nil {
		return
	}
	if frame.Type == "turn.completed" {
		m.history(shared.HistoryRecord{EpisodeID: task, Source: source, Kind: "run.usage", InputTokens: frame.Usage.Input, OutputTokens: frame.Usage.Output, UsageScope: "turn"})
		return
	}
	if frame.Type != "item.started" && frame.Type != "item.completed" {
		return
	}
	v := frame.Item
	tool := ""
	input, output := "", ""
	switch v.Type {
	case "command_execution":
		tool = "terminal"
		input = v.Command
		output = v.Output
	case "file_change":
		tool = "file_change"
		output = string(v.Changes)
	case "mcp_tool_call":
		tool = v.Name
		input = string(v.Arguments)
		output = string(v.Result)
	default:
		return
	}
	if input != "" {
		m.history(shared.HistoryRecord{ID: stableHistoryID(task, "tool.call", v.ID), EpisodeID: task, Source: source, Kind: "tool.call", Role: "assistant", Tool: tool, ToolCallID: v.ID, Content: input})
	}
	if frame.Type == "item.completed" {
		m.history(shared.HistoryRecord{ID: stableHistoryID(task, "tool.result", v.ID), EpisodeID: task, Source: source, Kind: "tool.result", Role: "tool", Tool: tool, ToolCallID: v.ID, Content: output, Status: strings.ToLower(v.Status)})
	}
}
