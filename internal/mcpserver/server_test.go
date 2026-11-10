package mcpserver

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type authTransport struct{ token string }

func (a authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+a.token)
	return http.DefaultTransport.RoundTrip(r)
}
func setup(t *testing.T, up http.Handler) (*mcp.ClientSession, *httptest.Server) {
	t.Helper()
	cp := httptest.NewServer(up)
	t.Cleanup(cp.Close)
	h, err := New(Config{ControlplaneURL: cp.URL, ControlToken: "control-test", MCPToken: "caller-test"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: authTransport{"caller-test"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session, srv
}
func call(t *testing.T, s *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	r, e := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestSDKTools(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	s, _ := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, err := shared.VerifyProjectAccess("control-test", strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), time.Now()); err != nil || p != "default" {
			t.Error("MCP must forward a project credential, never global control authority")
		}
		mu.Lock()
		seen[r.Method+" "+r.URL.RequestURI()]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			var b map[string]string
			if json.NewDecoder(r.Body).Decode(&b) != nil {
				t.Error("bad JSON")
			}
			if b["idempotency_key"] != "key-1" {
				t.Error("missing key")
			}
			if r.URL.Path == "/v1/workspaces" && b["project_id"] != "default" {
				t.Error("wrong project")
			}
			if strings.HasSuffix(r.URL.Path, "/tasks") && b["instruction"] != "hello" {
				t.Error("wrong instruction")
			}
		}
		switch {
		case r.URL.Path == "/v1/usage":
			fmt.Fprint(w, `{"running_workspaces":1,"cpu_quota":4}`)
		case strings.HasSuffix(r.URL.Path, "/result"):
			fmt.Fprint(w, `{"task_id":"t1","state":"succeeded","content":"done","next_offset":4}`)
		case strings.Contains(r.URL.Path, "/tasks"):
			fmt.Fprint(w, `{"id":"t1","workspace_id":"w1","state":"running"}`)
		case r.Method == "GET" && r.URL.Path == "/v1/workspaces":
			fmt.Fprint(w, `{"items":[{"id":"w1","cpu_cores":null,"memory_bytes":null}]}`)
		default:
			fmt.Fprint(w, `{"id":"w1","project_id":"default","state":"READY","cpu_cores":null}`)
		}
	}))
	list, e := s.ListTools(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(list.Tools) != 28 {
		t.Fatalf("tools=%d", len(list.Tools))
	}
	for _, tool := range list.Tools {
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("missing schemas: %s", tool.Name)
		}
	}
	cases := []struct {
		name  string
		args  map[string]any
		route string
	}{
		{"workspace_list", map[string]any{}, "GET /v1/workspaces"},
		{"workspace_get", map[string]any{"workspace_id": "w1"}, "GET /v1/workspaces/w1"},
		{"workspace_create", map[string]any{"idempotency_key": "key-1"}, "POST /v1/workspaces"},
		{"workspace_start", map[string]any{"workspace_id": "w1", "idempotency_key": "key-1"}, "POST /v1/workspaces/w1/start"},
		{"workspace_stop", map[string]any{"workspace_id": "w1", "idempotency_key": "key-1"}, "POST /v1/workspaces/w1/stop"},
		{"task_submit", map[string]any{"workspace_id": "w1", "idempotency_key": "key-1", "instruction": "hello"}, "POST /v1/workspaces/w1/tasks"},
		{"task_status", map[string]any{"workspace_id": "w1", "task_id": "t1"}, "GET /v1/workspaces/w1/tasks/t1"},
		{"task_result", map[string]any{"workspace_id": "w1", "task_id": "t1"}, "GET /v1/workspaces/w1/tasks/t1/result?offset=0&limit=65536"},
		{"task_cancel", map[string]any{"workspace_id": "w1", "task_id": "t1", "idempotency_key": "key-1"}, "POST /v1/workspaces/w1/tasks/t1/cancel"},
		{"usage_current", map[string]any{}, "GET /v1/usage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := call(t, s, tc.name, tc.args)
			if r.IsError || r.StructuredContent == nil {
				t.Fatalf("result: %+v", r)
			}
			mu.Lock()
			n := seen[tc.route]
			mu.Unlock()
			if n != 1 {
				t.Fatalf("route count %d", n)
			}
			if tc.name == "task_result" {
				b, _ := json.Marshal(r.StructuredContent)
				if !strings.Contains(string(b), `"content":"done"`) {
					t.Fatal(string(b))
				}
			}
		})
	}
}

func TestInvalidArguments(t *testing.T) {
	s, _ := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid input reached upstream")
		w.WriteHeader(500)
	}))
	cases := []struct {
		name string
		args map[string]any
	}{
		{"workspace_get", map[string]any{"workspace_id": "../x"}},
		{"workspace_get", map[string]any{"workspace_id": "x%2Fy"}},
		{"workspace_get", map[string]any{"workspace_id": "x\\y"}},
		{"workspace_get", map[string]any{"workspace_id": strings.Repeat("a", 129)}},
		{"task_status", map[string]any{"workspace_id": "w1", "task_id": "a/b"}},
		{"workspace_create", map[string]any{"idempotency_key": ""}},
		{"task_submit", map[string]any{"workspace_id": "w1", "instruction": strings.Repeat("a", MaxInstructionBytes+1), "idempotency_key": "key"}},
		{"task_submit", map[string]any{"workspace_id": "w1", "instruction": "hello", "idempotency_key": "key", "provider": "arbitrary-command"}},
		{"task_result", map[string]any{"workspace_id": "w1", "task_id": "t1", "offset": -1}},
		{"task_result", map[string]any{"workspace_id": "w1", "task_id": "t1", "limit": MaxResultBytes + 1}},
	}
	for _, tc := range cases {
		if r := call(t, s, tc.name, tc.args); !r.IsError {
			t.Fatalf("accepted invalid %s", tc.name)
		}
	}
}

func TestNativeProviderForwarding(t *testing.T) {
	s, _ := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]string
		if json.NewDecoder(r.Body).Decode(&input) != nil || input["provider"] != "claude" || input["instruction"] != "implement" {
			t.Error("native provider was lost")
		}
		fmt.Fprint(w, `{"id":"t1","workspace_id":"w1","state":"queued"}`)
	}))
	if r := call(t, s, "task_submit", map[string]any{"workspace_id": "w1", "idempotency_key": "native-key", "instruction": "implement", "provider": "claude"}); r.IsError {
		t.Fatal("native provider rejected")
	}
}
func TestHTTPGuards(t *testing.T) {
	h, e := New(Config{ControlplaneURL: "http://controlplane:8080", ControlToken: "cp", MCPToken: "mcp", AllowedHosts: []string{"localhost:8080", "127.0.0.1", "mcp"}})
	if e != nil {
		t.Fatal(e)
	}
	cases := []struct {
		path, host, origin, token string
		want                      int
	}{
		{"/healthz", "evil.example", "", "", 200},
		{"/mcp", "localhost:8080", "", "", 401},
		{"/mcp", "localhost:8080", "", "Bearer cp", 401},
		{"/mcp", "evil.example", "", "Bearer mcp", 403},
		{"/mcp", "localhost:8081", "", "Bearer mcp", 403},
		{"/mcp", "localhost:8080", "http://evil.example", "Bearer mcp", 403},
		{"/mcp", "localhost:8080", "null", "Bearer mcp", 403},
		{"/mcp", "localhost:8080", "http://localhost:8081", "Bearer mcp", 403},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Host = tc.host
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		r.Header.Set("Authorization", tc.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%+v: %d", tc, w.Code)
		}
	}
	for _, host := range []string{"localhost:8080", "127.0.0.1:9999", "mcp:8080"} {
		if !allowedAuthority(host, []string{"localhost:8080", "127.0.0.1", "mcp"}) {
			t.Fatal(host)
		}
	}
}
func TestSafeUpstreamErrorsAndRetries(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	s, _ := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		w.WriteHeader(503)
		fmt.Fprint(w, `{"code":"NOT_READY","message":"secret internal detail"}`)
	}))
	r := call(t, s, "workspace_start", map[string]any{"workspace_id": "w1", "idempotency_key": "same-key"})
	b, _ := json.Marshal(r)
	if !r.IsError || !strings.Contains(string(b), "NOT_READY") || strings.Contains(string(b), "secret") {
		t.Fatalf("unsafe result %s", b)
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts != 2 {
		t.Fatalf("attempts=%d", attempts)
	}
}
func TestResultBoundAndRedirect(t *testing.T) {
	for _, mode := range []string{"oversize", "redirect", "raw-error"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "oversize":
					json.NewEncoder(w).Encode(Result{TaskID: "t1", Content: strings.Repeat("a", MaxResultBytes+1)})
				case "redirect":
					w.Header().Set("Location", "http://evil.example")
					w.WriteHeader(302)
				case "raw-error":
					w.WriteHeader(500)
					io.WriteString(w, "secret internals")
				}
			}))
			r := call(t, s, "task_result", map[string]any{"workspace_id": "w1", "task_id": "t1"})
			b, _ := json.Marshal(r)
			if !r.IsError || strings.Contains(string(b), "secret") || strings.Contains(string(b), "evil.example") {
				t.Fatalf("unsafe result %s", b)
			}
		})
	}
}

func TestCredentialConfig(t *testing.T) {
	for _, c := range []Config{
		{ControlplaneURL: "http://cp", ControlToken: "same", MCPToken: "same"},
		{ControlplaneURL: "http://cp", ControlToken: "", MCPToken: "token"},
		{ControlplaneURL: "http://cp/arbitrary", ControlToken: "cp", MCPToken: "token"},
		{ControlplaneURL: "http://cp", ControlToken: "cp", MCPToken: "token", AllowedHosts: []string{"*"}},
	} {
		if _, err := New(c); err == nil {
			t.Fatalf("accepted invalid config")
		}
	}
}

func TestCancellationAndIdempotentRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		n := len(bodies)
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"id":"w1"}`)
	}))
	defer up.Close()
	c := &controlClient{base: up.URL, token: "cp", client: up.Client()}
	var out Workspace
	if e := c.do(context.Background(), "POST", "/v1/workspaces", map[string]string{"idempotency_key": "same-key"}, &out); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	if len(bodies) != 2 || bodies[0] != bodies[1] || !strings.Contains(bodies[0], "same-key") {
		t.Error("retry changed mutation")
	}
	mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := c.do(ctx, "GET", "/v1/workspaces", nil, &out); e == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestNoRetryConflict(t *testing.T) {
	calls := 0
	c := &controlClient{token: "cp"}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(409)
		fmt.Fprint(w, `{"code":"IDEMPOTENCY_CONFLICT","message":"sensitive"}`)
	}))
	defer up.Close()
	c.base = up.URL
	c.client = up.Client()
	var out Workspace
	e := c.do(context.Background(), "POST", "/v1/workspaces", map[string]string{"idempotency_key": "key"}, &out)
	if e == nil || e.Error() != "IDEMPOTENCY_CONFLICT" || calls != 1 {
		t.Fatalf("error=%v calls=%d", e, calls)
	}
}

func TestWorkspaceCreateSchemaHasNoRetiredProfileSelector(t *testing.T) {
	hits := 0
	session, _ := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"workspace_new","project_id":"default","state":"STOPPED"}`)
	}))
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "workspace_create" {
			found = true
			raw, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "profile_id") {
				t.Fatalf("retired selector remains in MCP schema: %s", raw)
			}
		}
	}
	if !found {
		t.Fatal("workspace_create missing")
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "workspace_create", Arguments: map[string]any{"idempotency_key": "new", "profile_id": "general"}})
	if err == nil && !result.IsError {
		t.Fatal("retired selector was accepted by MCP")
	}
	if hits != 0 {
		t.Fatal("invalid MCP create reached controlplane")
	}
}
