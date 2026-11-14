package workspace

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveApplicationLifecycleAndForwarding(t *testing.T) {
	m, err := New(Config{ID: "ws_application", DataDir: t.TempDir(), Token: "test-workspace-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	dir := filepath.Join(m.cfg.DataDir, "files", "site")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "server.py"), []byte(`import os
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
 def do_GET(self):
  self.send_response(200);self.send_header('Content-Type','text/html');self.end_headers();self.wfile.write(b'<h1>live app</h1>')
 def do_POST(self):
  body=self.rfile.read(int(self.headers.get('Content-Length','0')))
  self.send_response(201);self.send_header('Content-Type','text/plain');self.end_headers();self.wfile.write(body+b' '+self.path.encode())
HTTPServer(('127.0.0.1',int(os.environ['PORT'])),H).serve_forever()
`), 0600)
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	body := fmt.Sprintf(`{"application":{"directory":"site","command":"python3 server.py","port":%d},"ttl_seconds":3600,"idempotency_key":"app-one"}`, port)
	started := invoke(t, m.Handler(), "POST", "/v1/services", body)
	if started.Code != 201 {
		t.Fatal(started.Code, started.Body.String())
	}
	var app applicationView
	json.Unmarshal(started.Body.Bytes(), &app)
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		m.mu.Lock()
		app, err = m.applicationView(app.ID)
		m.mu.Unlock()
		if err == nil && app.Ready {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !app.Ready {
		t.Fatal("application failed to start", app.Logs)
	}
	if res := invoke(t, m.Handler(), "POST", "/v1/services", body); res.Code != 200 {
		t.Fatal("retry duplicated app", res.Code)
	}
	if res := invoke(t, m.Handler(), "POST", "/v1/services", strings.Replace(body, "python3 server.py", "python3 other.py", 1)); res.Code != 409 {
		t.Fatal("changed retry accepted", res.Code)
	}
	fetched := invoke(t, m.Handler(), "POST", "/v1/services/"+app.ID+"/fetch", `{"path":"api","query":"q=1","method":"POST","content_type":"text/plain","body":"aGVsbG8="}`)
	var response struct {
		Content []byte `json:"content"`
		Status  int    `json:"status"`
	}
	json.Unmarshal(fetched.Body.Bytes(), &response)
	if fetched.Code != 200 || response.Status != 201 || string(response.Content) != "hello /api?q=1" {
		t.Fatal("HTTP forwarding failed", fetched.Code, fetched.Body.String())
	}

	// A preview keeps the worker alive without blocking the next conversation turn.
	if m.Activity().BackgroundJobs == 0 {
		t.Fatal("preview did not hold workspace open")
	}
	handoff := invoke(t, m.Handler(), "POST", "/v1/files/write-new", `{"path":"followup.md","content":"Q2hhbmdlIHRoZSBhcHA="}`)
	if handoff.Code != 201 {
		t.Fatal("preview blocked follow-up instructions", handoff.Code, handoff.Body.String())
	}
	m.mu.Lock()
	pid := m.applications[app.ID].cmd.Process.Pid
	stat, _ := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	owned := m.ownedApplicationProcess(pid, string(stat))
	workActivity := m.workActivityLocked()
	m.mu.Unlock()
	if !owned || workActivity.BackgroundJobs != 0 {
		t.Fatal("managed app was treated as unrelated work")
	}
	submitted := invoke(t, m.Handler(), "POST", "/v1/tasks/from-file", `{"instruction_path":"followup.md","idempotency_key":"preview-followup"}`)
	if submitted.Code != 202 {
		t.Fatal("preview blocked task admission", submitted.Code, submitted.Body.String())
	}
	var task struct {
		ID string `json:"id"`
	}
	json.Unmarshal(submitted.Body.Bytes(), &task)
	waitTask(t, m, task.ID)
	revoked := invoke(t, m.Handler(), "POST", "/v1/services/"+app.ID+"/revoke", `{}`)
	if revoked.Code != 200 {
		t.Fatal(revoked.Code)
	}
	if res := invoke(t, m.Handler(), "POST", "/v1/services/"+app.ID+"/fetch", `{"path":""}`); res.Code != 410 {
		t.Fatal("revoked preview remained reachable", res.Code)
	}
	for end := time.Now().Add(3 * time.Second); ; {
		c, e := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 50*time.Millisecond)
		if e != nil {
			break
		}
		c.Close()
		if time.Now().After(end) {
			t.Fatal("stopping app left port listening")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, bad := range []string{strings.Replace(body, `"port":`+fmt.Sprint(port), `"port":3773`, 1), strings.Replace(body, `"directory":"site"`, `"directory":"../"`, 1)} {
		if res := invoke(t, m.Handler(), "POST", "/v1/services", bad); res.Code != 400 {
			t.Fatal("invalid application accepted", res.Code)
		}
	}
}
