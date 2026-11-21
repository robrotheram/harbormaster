package workspace

import (
	"agent-workbench/internal/shared"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func invoke(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer test-workspace-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func waitTask(t *testing.T, m *Manager, id string) shared.Task {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		task, e := m.task(id)
		if e == nil && !shared.Active(task.State) {
			return task
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("task did not finish")
	return shared.Task{}
}

func TestRunReportsMissingAgentSelection(t *testing.T) {
	inference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shared.Error(w, http.StatusConflict, "AGENT_SELECTION_REQUIRED")
	}))
	defer inference.Close()
	manager := &Manager{cfg: Config{InferenceURL: inference.URL, HTTP: inference.Client()}}
	result, code := manager.run(context.Background(), "task_test", "Build a landing page")
	if result != "" || code != "AGENT_SELECTION_REQUIRED" {
		t.Fatalf("expected a selection error, got result %q and code %q", result, code)
	}
}

func TestDurableAcceptanceToolsAndRestart(t *testing.T) {
	var calls atomic.Int32
	inference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-workspace-token" {
			t.Error("grant missing")
		}
		var request struct {
			Messages []Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		n := calls.Add(1)
		if n == 1 {
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"write1","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"answer.txt\",\"content\":\"workspace-owned-content\"}"}}]}}]}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"finished"}}]}`)
	}))
	defer inference.Close()
	cfg := Config{ID: "ws_test", HomeID: "home_test", Token: "test-workspace-token", DataDir: t.TempDir(), InferenceURL: inference.URL, Model: "fixture"}
	m, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	req := `{"instruction":"write the answer","idempotency_key":"task-request"}`
	first := invoke(t, m.Handler(), "POST", "/v1/tasks", req)
	if first.Code != 202 {
		t.Fatalf("submit: %d %s", first.Code, first.Body)
	}
	var task shared.Task
	_ = json.Unmarshal(first.Body.Bytes(), &task)
	if task.ID == "" {
		t.Fatal("no task id")
	}
	done := waitTask(t, m, task.ID)
	if done.State != "succeeded" {
		t.Fatalf("state: %+v", done)
	}
	replay := invoke(t, m.Handler(), "POST", "/v1/tasks", req)
	if replay.Code != 200 {
		t.Fatalf("replay: %d", replay.Code)
	}
	var repeated shared.Task
	_ = json.Unmarshal(replay.Body.Bytes(), &repeated)
	if repeated.ID != task.ID || calls.Load() != 2 {
		t.Fatal("retry executed task twice")
	}
	conflict := invoke(t, m.Handler(), "POST", "/v1/tasks", `{"instruction":"different","idempotency_key":"task-request"}`)
	if conflict.Code != 409 {
		t.Fatal("changed request must conflict")
	}
	content, e := os.ReadFile(filepath.Join(cfg.DataDir, "files/answer.txt"))
	if e != nil || string(content) != "workspace-owned-content" {
		t.Fatal("tool did not persist file")
	}
	if e = m.Close(); e != nil {
		t.Fatal(e)
	}
	m, e = New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	persisted, e := m.task(task.ID)
	if e != nil || persisted.State != "succeeded" {
		t.Fatal("task lost on restart")
	}
	result := invoke(t, m.Handler(), "GET", "/v1/tasks/"+task.ID+"/result?limit=3", "")
	if result.Code != 200 || !bytes.Contains(result.Body.Bytes(), []byte(`"next_offset":3`)) {
		t.Fatalf("result pagination: %s", result.Body)
	}
}
func TestCancellationAndAtomicQuiesce(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	inference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer inference.Close()
	defer close(release)
	m, e := New(Config{ID: "ws_cancel", HomeID: "home_test", Token: "test-workspace-token", DataDir: t.TempDir(), InferenceURL: inference.URL})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	response := invoke(t, m.Handler(), "POST", "/v1/tasks", `{"instruction":"wait","idempotency_key":"wait"}`)
	var task shared.Task
	_ = json.Unmarshal(response.Body.Bytes(), &task)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("not running")
	}
	if w := invoke(t, m.Handler(), "POST", "/v1/quiesce", ""); w.Code != 409 {
		t.Fatal("quiesced while running")
	}
	cancel := invoke(t, m.Handler(), "POST", "/v1/tasks/"+task.ID+"/cancel", `{"idempotency_key":"cancel"}`)
	if cancel.Code != 200 {
		t.Fatal(cancel.Body)
	}
	if done := waitTask(t, m, task.ID); done.State != "cancelled" {
		t.Fatalf("cancel state: %s", done.State)
	}
	if w := invoke(t, m.Handler(), "POST", "/v1/quiesce", ""); w.Code != 200 {
		t.Fatal("idle quiesce failed")
	}
	if w := invoke(t, m.Handler(), "POST", "/v1/tasks", `{"instruction":"new work","idempotency_key":"after-quiesce"}`); w.Code != 503 {
		t.Fatal("accepted work after quiesce")
	}
}
func TestToolConfinementAndCancellation(t *testing.T) {
	m, e := New(Config{ID: "ws_files", HomeID: "home_test", Token: "test-workspace-token", DataDir: t.TempDir(), InferenceURL: "http://unused"})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	outside := t.TempDir()
	if e = os.Symlink(outside, filepath.Join(m.cfg.DataDir, "files", "escape")); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"../outside.txt", "escape/secret.txt", "/tmp/absolute.txt"} {
		raw, _ := json.Marshal(map[string]string{"path": path, "content": "bad"})
		out := m.tool(context.Background(), "write_file", raw)
		if !strings.Contains(out, "error") {
			t.Fatalf("escaped root via %s", path)
		}
	}
	if e := syscall.Mkfifo(filepath.Join(m.cfg.DataDir, "files", "fifo"), 0600); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"read_file", "write_file", "list_files"} {
		out := m.tool(context.Background(), name, json.RawMessage(`{"path":"fifo","content":"x"}`))
		if !strings.Contains(out, "error") {
			t.Fatalf("accepted FIFO for %s", name)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	out := m.tool(ctx, "run_command", json.RawMessage(`{"command":"sleep 30"}`))
	if !strings.Contains(out, `"timed_out":true`) {
		t.Fatalf("command did not cancel: %s", out)
	}
}
func TestInterruptedTasksAreNotReplayed(t *testing.T) {
	cfg := Config{ID: "ws_restart", HomeID: "home_test", Token: "test-workspace-token", DataDir: t.TempDir()}
	m, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, e = m.db.Exec(`INSERT INTO tasks(id,idem_key,fingerprint,instruction,state,created_at,updated_at) VALUES('task_old','old','hash','do not replay','running',?,?)`, now, now)
	if e != nil {
		t.Fatal(e)
	}
	m.Close()
	m, e = New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	task, e := m.task("task_old")
	if e != nil || task.State != "failed" || task.ErrorCode != "INTERRUPTED" {
		t.Fatalf("restart state %+v %v", task, e)
	}
}
