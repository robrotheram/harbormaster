package dashboard

import (
	"agent-workbench/internal/shared"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHumanIngressUsesVerifiedPrincipalNotBrowserHeaders(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		human, err := shared.VerifyHumanAccess("control", strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), time.Now())
		if err != nil || human.Email != "alice@example.com" || human.ID == "user_forged" {
			t.Errorf("identity: %+v %v", human, err)
		}
		w.Write([]byte(`{"items":[]}`))
	}))
	defer upstream.Close()
	h, err := NewInteractiveHandler(upstream.URL, upstream.URL, "control")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://localhost/ui-api/projects", nil)
	r.Header.Set("Authorization", "Bearer browser-token")
	r.Header.Set("X-Workbench-Subscription-User", "user_forged")
	r = withHumanIdentity(r, "test-issuer", "alice", "alice@example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !called {
		t.Fatal(w.Code, w.Body)
	}
}
func TestHumanHealthSummaryExcludesGlobalOperatorData(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"generated_at":"2026-11-17T10:00:00Z","workspaces":{"items":[{"id":"private-worker"}]},"usage":{"cpu_allocated":999},"active_tasks":["private-task"],"services":{"items":[{"name":"Control plane","status":"healthy","reason_code":"PROCESS_READY"},{"name":"MCP","status":"unknown","reason_code":"private-address"},{"name":"private-project","status":"healthy"}]}}`))
	}))
	defer upstream.Close()
	h, _ := NewInteractiveHandler(upstream.URL, upstream.URL, "control")
	r := withHumanIdentity(httptest.NewRequest("GET", "http://localhost/dashboard-api/v1/snapshot", nil), "test", "alice", "alice@example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"healthy"`) {
		t.Fatal(w.Code, w.Body)
	}
	for _, secret := range []string{"private-", "workspaces", "active_tasks", "cpu_allocated"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("private data in health summary", w.Body)
		}
	}
	if !strings.Contains(w.Body.String(), `"scope":"services"`) {
		t.Fatal(w.Body)
	}
}

func TestHumanOverviewRoutesPreserveIdentityAndTaskStartPath(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/projects/prj_one/capabilities", ""},
		{"POST", "/board/work_one/start", `{"project_id":"prj_one","revision":1}`},
		{"POST", "/board/work_one", `{"project_id":"prj_one","revision":1,"stage":"planned","description":"` + strings.Repeat("x", 6000) + `"}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v1"+tc.path || r.Method != tc.method {
					t.Errorf("wrong proxy route: %s %s", r.Method, r.URL.Path)
				}
				human, err := shared.VerifyHumanAccess("control", strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), time.Now())
				if err != nil || human.Email != "alice@example.com" {
					t.Error("human authority lost", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"ok":true}`))
			}))
			defer upstream.Close()
			h, err := NewInteractiveHandler(upstream.URL, upstream.URL, "control")
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(tc.method, "http://localhost/ui-api"+tc.path, strings.NewReader(tc.body))
			r.Header.Set("Origin", "http://localhost")
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Workbench-Request", "1")
			r = withHumanIdentity(r, "test", "alice", "alice@example.com")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 200 || calls != 1 {
				t.Fatal(w.Code, w.Body, calls)
			}
		})
	}
}
