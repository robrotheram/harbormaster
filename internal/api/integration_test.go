package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-workbench/internal/dashboard"
	"agent-workbench/internal/gateway"
	"agent-workbench/internal/mcpserver"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"agent-workbench/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpAuth struct{}

func (mcpAuth) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer external-agent-token")
	return http.DefaultTransport.RoundTrip(r)
}

// This runs real Go handlers and durable workspace storage. The runtime is an
// in-process test adapter; this deliberately makes no Docker/Kata/network claim.
func TestMCPToWorkspaceThroughInferenceGateway(t *testing.T) {
	rt := &embeddedRuntime{base: t.TempDir(), managers: map[string]*workspace.Manager{}, servers: map[string]*httptest.Server{}}
	defer rt.Close()
	verify := func(ctx context.Context, token, audience string) (gateway.Identity, error) {
		rt.mu.Lock()
		defer rt.mu.Unlock()
		if token != "workspace-token" || audience != "inference" {
			return gateway.Identity{}, fmt.Errorf("denied")
		}
		for id := range rt.managers {
			return gateway.Identity{WorkspaceID: id, ProjectID: "default", PrincipalID: "local-operator"}, nil
		}
		return gateway.Identity{}, fmt.Errorf("not running")
	}
	gh, e := gateway.NewHandler(gateway.Config{Mode: "fixture", Models: []string{"fixture"}, Verify: verify})
	if e != nil {
		t.Fatal(e)
	}
	inference := httptest.NewServer(gh)
	defer inference.Close()
	rt.upstream = inference.URL
	metadata, e := store.Open(context.Background(), filepath.Join(t.TempDir(), "metadata.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer metadata.DB.Close()
	core := New(metadata, rt, Config{Token: "internal-control-token"})
	cp := httptest.NewServer(core.Handler())
	defer cp.Close()
	mh, e := mcpserver.New(mcpserver.Config{ControlplaneURL: cp.URL, ControlToken: "internal-control-token", MCPToken: "external-agent-token"})
	if e != nil {
		t.Fatal(e)
	}
	ms := httptest.NewServer(mh)
	defer ms.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, e := mcp.NewClient(&mcp.Implementation{Name: "workbench-integration-test", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ms.URL + "/mcp", HTTPClient: &http.Client{Transport: mcpAuth{}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	call := func(name string, args map[string]any, out any) {
		t.Helper()
		r, e := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if e != nil || r.IsError {
			b, _ := json.Marshal(r)
			t.Fatalf("%s: %s %v", name, b, e)
		}
		b, _ := json.Marshal(r.StructuredContent)
		if json.Unmarshal(b, out) != nil {
			t.Fatalf("invalid %s output", name)
		}
	}
	var ws shared.Workspace
	call("workspace_create", map[string]any{"idempotency_key": "integration-workspace"}, &ws)
	if ws.ID == "" {
		t.Fatal("no workspace")
	}
	var task shared.Task
	call("task_submit", map[string]any{"workspace_id": ws.ID, "idempotency_key": "integration-task", "instruction": "private-input-marker"}, &task)
	deadline := time.Now().Add(2 * time.Second)
	for shared.Active(task.State) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		call("task_status", map[string]any{"workspace_id": ws.ID, "task_id": task.ID}, &task)
	}
	if task.State != "succeeded" {
		t.Fatalf("task: %+v", task)
	}
	var result struct {
		Content string `json:"content"`
	}
	call("task_result", map[string]any{"workspace_id": ws.ID, "task_id": task.ID}, &result)
	if !strings.Contains(result.Content, "SYNTHETIC FIXTURE") {
		t.Fatal("gateway response missing")
	}
	read := httptest.NewServer(core.DashboardHandler())
	defer read.Close()
	dh, e := dashboard.NewHandler(read.URL, shared.DashboardReadToken("internal-control-token"))
	if e != nil {
		t.Fatal(e)
	}
	ui := httptest.NewServer(dh)
	defer ui.Close()
	response, e := http.Get(ui.URL + "/dashboard-api/v1/snapshot")
	if e != nil {
		t.Fatal(e)
	}
	snapshot, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(string(snapshot), ws.ID) || strings.Contains(string(snapshot), "private-input-marker") || strings.Contains(string(snapshot), "SYNTHETIC FIXTURE") {
		t.Fatal("dashboard projection failed")
	}
	var listed mcpserver.WorkspaceList
	call("workspace_list", map[string]any{}, &listed)
	if len(listed.Items) != 1 || listed.Items[0].IsolationStatus != "unverified" {
		t.Fatal("incorrect runtime claim")
	}
	call("workspace_stop", map[string]any{"workspace_id": ws.ID, "idempotency_key": "integration-stop"}, &ws)
	if ws.State != "STOPPED" {
		t.Fatal("stop failed")
	}
	call("task_result", map[string]any{"workspace_id": ws.ID, "task_id": task.ID}, &result)
	if !strings.Contains(result.Content, "SYNTHETIC FIXTURE") {
		t.Fatal("reconnect lost durable result")
	}
}
