package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"agent-workbench/internal/workspace"
)

// embeddedRuntime provides the full controller contract for API tests.
// Tests embed it and override the operations relevant to their scenario.
type embeddedRuntime struct {
	mu             sync.Mutex
	base, upstream string
	managers       map[string]*workspace.Manager
	servers        map[string]*httptest.Server
	starts         int
	unknown        bool
	jobs           int
}

func (rt *embeddedRuntime) StartConfigured(_ context.Context, id string, _ protocol.WorkspaceSpec) (shared.Observation, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.managers[id] == nil {
		m, e := workspace.New(workspace.Config{ID: id, HomeID: "home_" + id, DataDir: filepath.Join(rt.base, id), Token: "workspace-token", InferenceURL: rt.upstream, Model: "fixture"})
		if e != nil {
			return shared.Observation{}, e
		}
		rt.managers[id] = m
		rt.servers[id] = httptest.NewServer(m.Handler())
		rt.starts++
	}
	return rt.inspect(id), nil
}
func (rt *embeddedRuntime) inspect(id string) shared.Observation {
	o := shared.Observation{RuntimeType: "docker", IsolationStatus: "unverified", NetworkStatus: "unverified"}
	if m := rt.managers[id]; m != nil {
		o.Running = true
		o.Ready = true
		o.Activity = m.Activity()
		o.Activity.Known = !rt.unknown
		o.Activity.BackgroundJobs = rt.jobs
	}
	return o
}
func (rt *embeddedRuntime) Inspect(ctx context.Context, id string) (shared.Observation, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.inspect(id), nil
}
func (rt *embeddedRuntime) Stop(ctx context.Context, id string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if m := rt.managers[id]; m != nil {
		if rt.unknown || rt.jobs > 0 {
			return fmt.Errorf("unknown activity")
		}
		r := httptest.NewRequest("POST", "/v1/quiesce", nil)
		r.Header.Set("Authorization", "Bearer workspace-token")
		w := httptest.NewRecorder()
		m.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			return fmt.Errorf("busy")
		}
		rt.servers[id].Close()
		m.Close()
		delete(rt.servers, id)
		delete(rt.managers, id)
	}
	return nil
}
func (rt *embeddedRuntime) Agent(ctx context.Context, id, method, path string, b []byte) (int, []byte, error) {
	rt.mu.Lock()
	server := rt.servers[id]
	rt.mu.Unlock()
	if server == nil {
		return 0, nil, fmt.Errorf("not running")
	}
	req, _ := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer workspace-token")
	r, e := http.DefaultClient.Do(req)
	if e != nil {
		return 0, nil, e
	}
	defer r.Body.Close()
	body, e := io.ReadAll(r.Body)
	return r.StatusCode, body, e
}
func (rt *embeddedRuntime) Capabilities(context.Context) (protocol.RuntimeCapability, error) {
	return protocol.RuntimeCapability{
		SchemaVersion: 1,
		ID:            protocol.DevelopmentTemplate,
		Version:       "test",
		Backend:       "docker",
		AgentKinds:    []string{"t3", "codex"},
		NetworkModes:  []string{"blocked", "allowlist", "open"},
		Resources:     protocol.ResourceLimits{CPUCores: 1, MemoryBytes: 512 << 20},
	}, nil
}
func (rt *embeddedRuntime) Reconcile(context.Context, string) error { return nil }
func (rt *embeddedRuntime) ConfigureConnections(context.Context, string, shared.ConnectionPolicy) error {
	return nil
}
func (rt *embeddedRuntime) MatrixCredentialPresent(context.Context, string) bool { return false }
func (rt *embeddedRuntime) MatrixCredential(context.Context, string, shared.HermesMatrix, string) error {
	return fmt.Errorf("MATRIX_CREDENTIAL_REJECTED")
}
func (rt *embeddedRuntime) MatrixAction(context.Context, string, string, *shared.HermesMatrix) error {
	return fmt.Errorf("HERMES_SETUP_REQUIRED")
}
func (rt *embeddedRuntime) EnsureProjectNamespace(context.Context, string) (string, error) {
	return "", fmt.Errorf("PROJECT_NAMESPACE_UNAVAILABLE")
}
func (rt *embeddedRuntime) Desktop(w http.ResponseWriter, _ *http.Request, _, _ string) {
	shared.Error(w, 503, "CONSOLE_UNAVAILABLE")
}
func (rt *embeddedRuntime) Console(w http.ResponseWriter, _ *http.Request, _, _ string) {
	shared.Error(w, 503, "CONSOLE_UNAVAILABLE")
}
func (rt *embeddedRuntime) Application(w http.ResponseWriter, _ *http.Request, _, _ string) {
	shared.Error(w, 503, "APPLICATION_PROXY_UNAVAILABLE")
}

func (rt *embeddedRuntime) Close() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for id, m := range rt.managers {
		rt.servers[id].Close()
		m.Close()
	}
}
func setup(t *testing.T) (*Server, *embeddedRuntime) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"sensitive-result-marker"}}]}`)
	}))
	t.Cleanup(upstream.Close)
	db, e := store.Open(context.Background(), filepath.Join(t.TempDir(), "metadata.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.DB.Close() })
	rt := &embeddedRuntime{base: t.TempDir(), upstream: upstream.URL, managers: map[string]*workspace.Manager{}, servers: map[string]*httptest.Server{}}
	t.Cleanup(rt.Close)
	return New(db, rt, Config{Token: "control-token"}), rt
}
func request(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer control-token")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func create(t *testing.T, s *Server, key string) shared.Workspace {
	t.Helper()
	w := request(t, s, "POST", "/v1/workspaces", `{"project_id":"default","idempotency_key":"`+key+`"}`)
	if w.Code != 201 && w.Code != 200 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var ws shared.Workspace
	_ = json.Unmarshal(w.Body.Bytes(), &ws)
	return ws
}
func TestUnauthorizedAndConcurrentStart(t *testing.T) {
	s, rt := setup(t)
	r := httptest.NewRequest("POST", "/v1/workspaces", strings.NewReader(`{"idempotency_key":"no"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("unauthorized mutation accepted")
	}
	items, _ := s.Store.Workspaces(context.Background())
	if len(items) != 0 || rt.starts != 0 {
		t.Fatal("unauthorized allocation")
	}
	ws := create(t, s, "one")
	replay := create(t, s, "one")
	if ws.ID != replay.ID {
		t.Fatal("create replay duplicated")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", fmt.Sprintf(`{"idempotency_key":"start%d"}`, i))
			if w.Code != 200 {
				t.Errorf("start: %d %s", w.Code, w.Body)
			}
		}(i)
	}
	wg.Wait()
	if rt.starts != 1 {
		t.Fatalf("created %d runtimes", rt.starts)
	}
}
func TestTaskPersistenceAndMetadataBoundary(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "content")
	body := `{"instruction":"sensitive-instruction-marker","idempotency_key":"task1"}`
	first := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/tasks", body)
	if first.Code != 202 {
		t.Fatalf("task: %d %s", first.Code, first.Body)
	}
	var task shared.Task
	_ = json.Unmarshal(first.Body.Bytes(), &task)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r := request(t, s, "GET", "/v1/workspaces/"+ws.ID+"/tasks/"+task.ID, "")
		_ = json.Unmarshal(r.Body.Bytes(), &task)
		if task.State == "succeeded" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if task.State != "succeeded" {
		t.Fatal("task did not complete")
	}
	result := request(t, s, "GET", "/v1/workspaces/"+ws.ID+"/tasks/"+task.ID+"/result", "")
	if !strings.Contains(result.Body.String(), "sensitive-result-marker") {
		t.Fatal("result missing")
	}
	replay := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/tasks", body)
	var same shared.Task
	_ = json.Unmarshal(replay.Body.Bytes(), &same)
	if same.ID != task.ID {
		t.Fatal("duplicate task")
	}
	// Simulate API restart with the same durable metadata and active runtime.
	restarted := New(s.Store, rt, Config{Token: "control-token"})
	restarted.Reconcile(context.Background())
	if rt.starts != 1 {
		t.Fatal("reconciliation restarted workspace")
	}
	rows, e := s.Store.DB.Query(`SELECT metadata FROM workspaces UNION ALL SELECT metadata FROM task_metadata`)
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var metadata string
		rows.Scan(&metadata)
		if strings.Contains(metadata, "sensitive-") {
			t.Fatal("content leaked into platform database")
		}
	}
	rows.Close()
	snapshot := httptest.NewRecorder()
	restarted.DashboardHandler().ServeHTTP(snapshot, httptest.NewRequest("GET", "/dashboard-api/v1/snapshot", nil))
	if strings.Contains(snapshot.Body.String(), "sensitive-") {
		t.Fatal("content leaked to dashboard")
	}
	stop := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/stop", `{"idempotency_key":"stop"}`)
	if stop.Code != 200 {
		t.Fatalf("stop: %s", stop.Body)
	}
	persisted := request(t, s, "GET", "/v1/workspaces/"+ws.ID+"/tasks/"+task.ID+"/result", "")
	if persisted.Code != 200 || !strings.Contains(persisted.Body.String(), "sensitive-result-marker") {
		t.Fatal("result lost after workspace restart")
	}
}
func TestIdleStopUnknownActivityAndQuota(t *testing.T) {
	s, rt := setup(t)
	s.cfg.IdleTimeout = 15 * time.Minute
	s.cfg.MaxRunning = 1
	ws := create(t, s, "idle")
	other := create(t, s, "other")
	if w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"start"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := request(t, s, "POST", "/v1/workspaces/"+other.ID+"/start", `{"idempotency_key":"start"}`); w.Code != 429 {
		t.Fatal("quota not enforced")
	}
	s.now = func() time.Time { return time.Now().UTC().Add(16 * time.Minute) }
	rt.unknown = true
	s.Reconcile(context.Background())
	if rt.managers[ws.ID] == nil {
		t.Fatal("unknown activity stopped workspace")
	}
	rt.unknown = false
	rt.jobs = 1
	s.Reconcile(context.Background())
	if rt.managers[ws.ID] == nil {
		t.Fatal("background job stopped workspace")
	}
	rt.jobs = 0
	s.Reconcile(context.Background())
	if rt.managers[ws.ID] == nil {
		t.Fatal("did not allow full idle interval after background work")
	}
	s.now = func() time.Time { return time.Now().UTC().Add(32 * time.Minute) }
	s.Reconcile(context.Background())
	if rt.managers[ws.ID] != nil {
		t.Fatal("idle workspace did not stop")
	}
	if w := request(t, s, "POST", "/v1/workspaces/"+other.ID+"/start", `{"idempotency_key":"start-again"}`); w.Code != 200 {
		t.Fatalf("quota not released: %s", w.Body)
	}
}
