package firecracker

import (
	"agent-workbench/internal/mcpserver"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpTransport func(*http.Request) (*http.Response, error)

func (f mcpTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProjectMCPGenerationToolInvocation(t *testing.T) {
	const key = "host-only-signing-authority"
	calls := 0
	cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project, err := shared.VerifyProjectAccess(key, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), time.Now())
		if err != nil || project != "personal" || r.URL.Query().Get("project_id") != "personal" {
			http.Error(w, `{"code":"NOT_FOUND_OR_FORBIDDEN"}`, 404)
			return
		}
		calls++
		if calls == 1 {
			http.Error(w, `{"code":"RUNTIME_UNAVAILABLE"}`, 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[]}`))
	}))
	defer cp.Close()
	handler, err := mcpserver.New(mcpserver.Config{ControlplaneURL: cp.URL, ControlToken: key, MCPToken: "operator-only", AllowedHosts: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(handler)
	defer upstream.Close()
	client := &http.Client{Transport: mcpTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != projectMCPAddress {
			t.Fatalf("arbitrary target: %s", r.URL)
		}
		r.URL.Scheme = "http"
		r.URL.Host = strings.TrimPrefix(upstream.URL, "http://")
		r.Host = r.URL.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	v := &fcVM{ID: "workspace", Generation: "generation-1", running: true, Spec: protocol.WorkspaceSpec{AgentKind: "hermes", ProjectID: "personal"}}
	f := &Firecracker{secret: "grant-authority", vms: map[string]*fcVM{"workspace": v}}
	broker := httptest.NewServer(f.projectMCP(v, key, client))
	defer broker.Close()
	guest := &http.Client{Transport: mcpTransport(func(r *http.Request) (*http.Response, error) {
		r.Header.Set("Authorization", "Bearer "+f.grant(v))
		return http.DefaultTransport.RoundTrip(r)
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "Hermes-fixture", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: broker.URL + "/mcp", HTTPClient: guest}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 19 {
		t.Fatalf("narrow discovery: %v %v", tools, err)
	}
	for _, tool := range tools.Tools {
		if strings.HasPrefix(tool.Name, "factory_job_") {
			t.Fatalf("retired project tool advertised: %s", tool.Name)
		}
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "work_list", Arguments: map[string]string{"project_id": "personal"}})
	if err != nil || result.IsError || calls != 2 {
		t.Fatalf("retry invocation: %v %v calls=%d", result, err, calls)
	}
	for _, name := range []string{"board_list", "board_create", "board_update"} {
		if !shared.ProjectToolAllowed(name) {
			t.Fatal("missing board permission", name)
		}
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "board_list", Arguments: map[string]string{"project_id": "personal"}})
	if err != nil || result.IsError {
		t.Fatal("board listing denied", result, err)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "board_list", Arguments: map[string]string{"project_id": "foreign"}})
	if err != nil || !result.IsError {
		t.Fatal("foreign board exposed", result, err)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "work_list", Arguments: map[string]string{"project_id": "foreign"}})
	if err != nil || !result.IsError {
		t.Fatalf("foreign project accepted: %v %v", result, err)
	}

	if source := os.Getenv("HERMES_SOURCE"); source != "" {
		integration, _ := filepath.Abs("../../../integrations/hermes")
		cmd := exec.CommandContext(ctx, filepath.Join(source, ".venv/bin/python"), "-m", "workbench_hermes_tests.live_discovery")
		cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(integration, "src")+string(os.PathListSeparator)+filepath.Join(integration, "tests")+string(os.PathListSeparator)+source, "TEST_MCP_ENDPOINT="+broker.URL+"/mcp", "TEST_MCP_GRANT="+f.grant(v))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("pinned Hermes: %s %v", out, err)
		}
	}
	f.mu.Lock()
	v.stopping = true
	f.mu.Unlock()
	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "work_list", Arguments: map[string]string{"project_id": "personal"}})
	if err == nil {
		t.Fatal("revoked generation accepted")
	}
	expired, _ := shared.IssueProjectAccess(key, "personal", time.Now().Add(-time.Second))
	if _, err := shared.VerifyProjectAccess(key, expired, time.Now()); err == nil {
		t.Fatal("expired credential accepted")
	}
}
