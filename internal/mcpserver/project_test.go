package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-workbench/internal/shared"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSignedProjectCredentialNeverBecomesOperatorAuthority(t *testing.T) {
	credential, _ := shared.IssueProjectAccess("control-secret", "project_a", time.Now().Add(time.Hour))
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project, err := shared.VerifyProjectAccess("control-secret", strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), time.Now())
		if err != nil || project != "project_a" || r.Header.Get("Authorization") != "Bearer "+credential {
			t.Error("project authority was replaced")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[{"id":"project_a","name":"A"}]}`)
	}))
	defer up.Close()
	h, err := New(Config{ControlplaneURL: up.URL, ControlToken: "control-secret", MCPToken: "legacy-secret"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "project-test", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: authTransport{credential}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "project_list", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatal("signed project discovery failed")
	}
}
