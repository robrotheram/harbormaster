package mcpserver

import (
	"context"
	"net/url"
	"time"

	"agent-workbench/internal/shared"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ProjectList struct {
	Items []shared.Project `json:"items"`
}
type ScheduleList struct {
	Items []shared.Schedule `json:"items"`
}
type RunList struct {
	Items []shared.Run `json:"items"`
}
type ProjectFilter struct {
	ProjectID string `json:"project_id,omitempty"`
}
type ScheduleInput struct {
	ProjectID       string    `json:"project_id"`
	WorkspaceID     string    `json:"workspace_id"`
	Name            string    `json:"name"`
	InstructionPath string    `json:"instruction_path"`
	Provider        string    `json:"provider,omitempty"`
	RunAt           time.Time `json:"run_at"`
	IntervalSeconds int64     `json:"interval_seconds,omitempty"`
	IdempotencyKey  string    `json:"idempotency_key"`
}
type ScheduleAction struct {
	ScheduleID     string `json:"schedule_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

func registerOrchestrationTools(s *mcp.Server, c *controlClient) {
	add(s, "project_list", "List only projects authorized by this connection's project credential.", func(ctx context.Context, _ Empty) (out ProjectList, err error) {
		err = c.do(ctx, "GET", "/v1/projects", nil, &out)
		return
	})
	add(s, "schedule_list", "List schedules in the authorized project.", func(ctx context.Context, in ProjectFilter) (out ScheduleList, err error) {
		err = c.do(ctx, "GET", "/v1/schedules?project_id="+url.QueryEscape(in.ProjectID), nil, &out)
		return
	})
	add(s, "schedule_create", "Schedule a native Codex or project Hermes task from a project-local instruction file. No API billing fallback.", func(ctx context.Context, in ScheduleInput) (out shared.Schedule, err error) {
		err = c.do(ctx, "POST", "/v1/schedules", in, &out)
		return
	})
	add(s, "run_list", "List scheduled execution metadata in the authorized project.", func(ctx context.Context, in ProjectFilter) (out RunList, err error) {
		err = c.do(ctx, "GET", "/v1/runs?project_id="+url.QueryEscape(in.ProjectID), nil, &out)
		return
	})
	add(s, "schedule_run_now", "Queue one idempotent schedule occurrence.", func(ctx context.Context, in ScheduleAction) (out shared.Run, err error) {
		if !identifier.MatchString(in.ScheduleID) || !keyPattern.MatchString(in.IdempotencyKey) {
			return out, invalid
		}
		err = c.do(ctx, "POST", "/v1/schedules/"+in.ScheduleID+"/run-now", map[string]string{"idempotency_key": in.IdempotencyKey}, &out)
		return
	})
	for _, action := range []string{"enable", "disable"} {
		add(s, "schedule_"+action, "Change whether future schedule occurrences are enabled.", func(ctx context.Context, in ScheduleAction) (out shared.Schedule, err error) {
			if !identifier.MatchString(in.ScheduleID) || !keyPattern.MatchString(in.IdempotencyKey) {
				return out, invalid
			}
			err = c.do(ctx, "POST", "/v1/schedules/"+in.ScheduleID+"/"+action, map[string]string{"idempotency_key": in.IdempotencyKey}, &out)
			return
		})
	}
}
