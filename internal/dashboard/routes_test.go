package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIRouteBoundary(t *testing.T) {
	for _, tt := range []struct {
		method, path string
		allowed      bool
	}{
		{"POST", "/projects/p_one/hermes/assistant", true},
		{"GET", "/projects/p_one/hermes/assistant", false},
		{"POST", "/projects/p_one/hermes/assistant/extra", false},
		{"POST", "/projects/p_one/hermes/matrix/start", true},
		{"GET", "/workspaces/ws_one/history/export", true},
		{"POST", "/workspaces/ws_one/history/purpose", true},
		{"POST", "/workspaces/ws_one/tasks/task_one/cancel", true},
		{"GET", "/workspaces/ws_one/tasks/task_one/result", true},
		{"POST", "/workspaces/ws_one/tasks/task_one/result", false},
		{"GET", "/workspaces/ws_one/agent/v1/tasks", false},
		{"POST", "/projects/p_one/agent-access", false},
		{"POST", "/workspaces/ws_one/network-policy", false},
		{"POST", "/grants/verify", false},
		{"DELETE", "/workspaces", false},
		{"GET", "/projects/../hermes", false},
		{"GET", "/workspaces//logs", false},
		{"GET", "/workspaces/", false},
		{"GET", "workspaces", false},
	} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			if got := allowedUIRoute(tt.method, tt.path); got != tt.allowed {
				t.Fatalf("allowed = %v, want %v", got, tt.allowed)
			}
		})
	}
}

func TestAssistantIngressKeepsOperatorAuthority(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/projects/p_one/hermes/assistant" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer private-operator" {
			t.Error("assistant request lost its bounded route or authority")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"ws_one","project_id":"p_one","state":"READY"}`))
	}))
	defer upstream.Close()
	handler, err := NewInteractiveHandler(upstream.URL, upstream.URL, "private-operator")
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"https://foreign.example", "http://127.0.0.1:8080"} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8080/ui-api/projects/p_one/hermes/assistant", strings.NewReader(`{"idempotency_key":"assistant-open"}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Workbench-Request", "1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		want := 200
		if strings.Contains(origin, "foreign") {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("origin %s: status %d", origin, w.Code)
		}
	}
	if calls != 1 {
		t.Fatalf("untrusted request reached control plane: %d calls", calls)
	}
}
