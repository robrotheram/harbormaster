package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func add[I, O any](s *mcp.Server, name, description string, fn func(context.Context, I) (O, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: description}, func(ctx context.Context, _ *mcp.CallToolRequest, in I) (*mcp.CallToolResult, O, error) {
		out, err := fn(ctx, in)
		return nil, out, err
	})
}
func workspacePath(id string) (string, error) {
	if !identifier.MatchString(id) {
		return "", invalid
	}
	return "/v1/workspaces/" + id, nil
}
func taskPath(w, t string) (string, error) {
	p, e := workspacePath(w)
	if e != nil || !identifier.MatchString(t) {
		return "", invalid
	}
	return p + "/tasks/" + t, nil
}
func keyBody(key string) (map[string]string, error) {
	if !keyPattern.MatchString(key) {
		return nil, invalid
	}
	return map[string]string{"idempotency_key": key}, nil
}

func registerTools(s *mcp.Server, c *controlClient) {
	registerOrchestrationTools(s, c)
	add(s, "workspace_list", "List workspaces in this connection's authorized project.", func(ctx context.Context, _ Empty) (out WorkspaceList, err error) {
		err = c.do(ctx, "GET", "/v1/workspaces", nil, &out)
		return
	})
	add(s, "workspace_get", "Read a workspace's status and measured usage.", func(ctx context.Context, in WorkspaceInput) (out Workspace, err error) {
		p, err := workspacePath(in.WorkspaceID)
		if err == nil {
			err = c.do(ctx, "GET", p, nil, &out)
		}
		return
	})
	add(s, "workspace_create", "Create a workspace in the authorized project. Reuse the idempotency key when retrying.", func(ctx context.Context, in CreateInput) (out Workspace, err error) {
		b, err := keyBody(in.IdempotencyKey)
		if err == nil {
			b["project_id"] = in.ProjectID
			if b["project_id"] == "" {
				if access, ok := ctx.Value(accessContextKey{}).(projectAccess); ok {
					b["project_id"] = access.project
				} else {
					b["project_id"] = "default"
				}
			}
			b["agent_kind"] = in.AgentKind
			b["name"], b["parent_id"], b["branch"] = in.Name, in.ParentID, in.Branch
			err = c.do(ctx, "POST", "/v1/workspaces", b, &out)
		}
		return
	})
	for _, action := range []string{"start", "stop"} {
		add(s, "workspace_"+action, "Request workspace "+action+". Reuse the idempotency key when retrying.", func(ctx context.Context, in MutationInput) (out Workspace, err error) {
			p, err := workspacePath(in.WorkspaceID)
			if err != nil {
				return
			}
			b, err := keyBody(in.IdempotencyKey)
			if err == nil {
				err = c.do(ctx, "POST", p+"/"+action, b, &out)
			}
			return
		})
	}
	add(s, "task_submit", "Submit an instruction, optionally choosing native subscription agent codex or claude. Omitted provider uses the deployment default. Returns task metadata; read output separately with task_result.", func(ctx context.Context, in SubmitInput) (out Task, err error) {
		p, err := workspacePath(in.WorkspaceID)
		if err != nil {
			return
		}
		b, err := keyBody(in.IdempotencyKey)
		if err != nil {
			return
		}
		if len(in.Instruction) > MaxInstructionBytes || strings.TrimSpace(in.Instruction) == "" || !utf8.ValidString(in.Instruction) {
			return out, invalid
		}
		b["instruction"] = in.Instruction
		if in.Provider != "" {
			if in.Provider != "codex" && in.Provider != "claude" {
				return out, invalid
			}
			b["provider"] = in.Provider
		}
		err = c.do(ctx, "POST", p+"/tasks", b, &out)
		return
	})
	add(s, "task_status", "Refresh task metadata from its workspace.", func(ctx context.Context, in TaskInput) (out Task, err error) {
		p, err := taskPath(in.WorkspaceID, in.TaskID)
		if err == nil {
			err = c.do(ctx, "GET", p, nil, &out)
		}
		return
	})
	add(s, "task_result", "Read a bounded page of task output. This tool exposes workspace content to the local operator.", func(ctx context.Context, in ResultInput) (out Result, err error) {
		p, err := taskPath(in.WorkspaceID, in.TaskID)
		if err != nil {
			return
		}
		if in.Limit == 0 {
			in.Limit = MaxResultBytes
		}
		if in.Offset < 0 || in.Limit < 1 || in.Limit > MaxResultBytes {
			return out, invalid
		}
		err = c.do(ctx, "GET", fmt.Sprintf("%s/result?offset=%d&limit=%d", p, in.Offset, in.Limit), nil, &out)
		if err == nil && (len(out.Content) > in.Limit || (out.NextOffset != -1 && out.NextOffset < in.Offset) || out.TaskID != in.TaskID) {
			out = Result{}
			err = fmt.Errorf("AGENT_PROTOCOL_UNAVAILABLE")
		}
		return
	})
	add(s, "task_cancel", "Request task cancellation. Reuse the idempotency key when retrying.", func(ctx context.Context, in CancelInput) (out Task, err error) {
		p, err := taskPath(in.WorkspaceID, in.TaskID)
		if err != nil {
			return
		}
		b, err := keyBody(in.IdempotencyKey)
		if err == nil {
			err = c.do(ctx, "POST", p+"/cancel", b, &out)
		}
		return
	})
	add(s, "usage_current", "Read current aggregate usage and quotas.", func(ctx context.Context, _ Empty) (out Usage, err error) {
		err = c.do(ctx, "GET", "/v1/usage", nil, &out)
		return
	})
}
