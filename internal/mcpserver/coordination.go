package mcpserver

import (
	"agent-workbench/internal/coordination"
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/url"
	"strconv"
)

// Register separately from general tools: assistant credentials must be bound to
// one project and explicit endpoint capabilities. Grant administration is human-only.
func registerCoordinationTools(s *mcp.Server, c *controlClient) {
	type publish struct {
		ProjectID  string `json:"project_id"`
		ExchangeID string `json:"exchange_id"`
		Occurrence string `json:"occurrence"`
		Body       string `json:"body"`
	}
	add(s, "exchange_publish", "Export explicitly selected content under a bilateral grant; occurrence makes retries idempotent.", func(ctx context.Context, in publish) (out coordination.Export, err error) {
		err = c.do(ctx, "POST", "/v1/project-exchanges/"+url.PathEscape(in.ExchangeID)+"/exports", map[string]string{"project_id": in.ProjectID, "occurrence": in.Occurrence, "body": in.Body}, &out)
		return
	})
	type inbox struct {
		ProjectID string `json:"project_id"`
		After     string `json:"after,omitempty"`
		Limit     int    `json:"limit,omitempty"`
	}
	add(s, "exchange_inbox", "Discover pending selected exports for this project; follow next through all pages.", func(ctx context.Context, in inbox) (out coordination.InboxPage, err error) {
		if in.Limit == 0 {
			in.Limit = 50
		}
		err = c.do(ctx, "GET", "/v1/exchange-inbox?project_id="+url.QueryEscape(in.ProjectID)+"&after="+url.QueryEscape(in.After)+"&limit="+strconv.Itoa(in.Limit), nil, &out)
		return
	})
	type read struct {
		ProjectID string `json:"project_id"`
		ExportID  string `json:"export_id"`
	}
	type result struct {
		Export coordination.Export `json:"export"`
		Body   string              `json:"body"`
	}
	add(s, "exchange_read", "Read one selected export. Revocation is checked on every read.", func(ctx context.Context, in read) (out result, err error) {
		err = c.do(ctx, "GET", "/v1/exchange-exports/"+url.PathEscape(in.ExportID)+"?project_id="+url.QueryEscape(in.ProjectID), nil, &out)
		return
	})
	add(s, "exchange_submit", "Submit a granted selected work request through the factory work API, with occurrence idempotency.", func(ctx context.Context, in read) (out map[string]any, err error) {
		err = c.do(ctx, "POST", "/v1/exchange-exports/"+url.PathEscape(in.ExportID)+"/submit", map[string]string{"project_id": in.ProjectID}, &out)
		return
	})

	type note struct {
		ProjectID   string                 `json:"project_id"`
		WorkspaceID string                 `json:"workspace_id"`
		Write       coordination.NoteWrite `json:"write"`
	}
	add(s, "note_publish", "Publish to an explicitly configured notes directory using an expected digest and occurrence ID.", func(ctx context.Context, in note) (out coordination.NoteReceipt, err error) {
		err = c.do(ctx, "POST", "/v1/note-publications", in, &out)
		return
	})
}
