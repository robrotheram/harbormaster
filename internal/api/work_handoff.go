package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"agent-workbench/internal/shared"
)

var handoffHistoryUnavailable = shared.CodedError{Status: http.StatusConflict, Code: "HANDOFF_HISTORY_UNAVAILABLE"}

func (s *Server) workHandoffContext(ctx context.Context, attempts []shared.WorkAttempt, expected string) ([]byte, error) {
	const maxContext = 512 << 10
	const maxAttempts = 32
	const maxMessages = 32
	slices.SortFunc(attempts, func(a, b shared.WorkAttempt) int {
		if byTime := b.CreatedAt.Compare(a.CreatedAt); byTime != 0 {
			return byTime
		}
		if a.ID == expected && b.ID != expected {
			return -1
		}
		if b.ID == expected && a.ID != expected {
			return 1
		}
		return strings.Compare(b.ID, a.ID)
	})
	type turn struct {
		Role      string `json:"role"`
		Text      string `json:"text"`
		Truncated bool   `json:"truncated,omitempty"`
	}
	var sections [][]byte
	size := 0
	missing := len(attempts) > maxAttempts
	for _, attempt := range attempts[:min(len(attempts), maxAttempts)] {
		if attempt.TaskID == "" {
			if attempt.ID == expected {
				return nil, handoffHistoryUnavailable
			}
			continue
		}
		records, err := s.Store.CachedMessages(ctx, attempt.WorkspaceID, attempt.TaskID, maxMessages+1)
		if err != nil {
			return nil, err
		}
		if attempt.ID == expected {
			ended, err := s.Store.CachedRunEnded(ctx, attempt.WorkspaceID, attempt.TaskID)
			if err != nil {
				return nil, err
			}
			missing = missing || !ended
		}
		if len(records) > maxMessages {
			records, missing = records[:maxMessages], true
		}
		hasRequest := false
		var encoded []byte
		for _, record := range records {
			if record.Kind == "user.message" && record.Content != "" {
				hasRequest = true
			}
			role := "assistant"
			if record.Kind == "user.message" {
				role = "user"
			}
			data, err := json.Marshal(turn{Role: role, Text: record.Content, Truncated: record.Truncated})
			if err != nil {
				return nil, err
			}
			if size+len(encoded)+len(data)+1 > maxContext-1024 {
				missing = true
				break
			}
			encoded = append(encoded, data...)
			encoded = append(encoded, '\n')
		}
		if !hasRequest {
			if attempt.ID == expected {
				return nil, handoffHistoryUnavailable
			}
			missing = true
		}
		if len(encoded) > 0 {
			sections = append(sections, encoded)
			size += len(encoded)
		}
	}
	if len(sections) == 0 {
		return nil, handoffHistoryUnavailable
	}
	var contextFile bytes.Buffer
	contextFile.WriteString("# Recorded conversation\n\nEarlier turns in chronological order. Each JSON line is one message.\n")
	if missing {
		contextFile.WriteString("Some earlier messages were unavailable or exceeded the transfer limit.\n")
	}
	contextFile.WriteString("\n")
	for i := len(sections) - 1; i >= 0; i-- {
		contextFile.Write(sections[i])
	}
	return contextFile.Bytes(), nil
}
