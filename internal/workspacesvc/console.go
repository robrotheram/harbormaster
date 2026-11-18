package workspacesvc

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"

	"time"
)

func (s *Service) RestoreConsole(ctx context.Context, ws shared.Workspace) error {
	if !ws.ConsoleEnabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	status, _, err := s.Runtime.Agent(ctx, ws.ID, "POST", "/v1/console/enable", nil)
	if status == 409 {
		return shared.CodedError{Status: shared.StatusConflict, Code: "WORKSPACE_BUSY"}
	}
	if err != nil || status != 200 {
		return shared.CodedError{Status: shared.StatusServiceUnavailable, Code: "CONSOLE_UNAVAILABLE"}
	}
	for {
		status, body, err := s.Runtime.Agent(ctx, ws.ID, "GET", "/v1/activity", nil)
		var a shared.Activity
		if err == nil && status == 200 && json.Unmarshal(body, &a) == nil && a.Known {
			return nil
		}
		select {
		case <-ctx.Done():
			return shared.CodedError{Status: shared.StatusServiceUnavailable, Code: "CONSOLE_NOT_READY"}
		case <-time.After(200 * time.Millisecond):
		}
	}
}
