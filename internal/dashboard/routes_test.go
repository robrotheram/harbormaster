package dashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-workbench/internal/shared"
)

func TestUIRouteBoundary(t *testing.T) {
	for _, tt := range []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/projects/project_one/capabilities", true},
		{"POST", "/projects/project_one/capabilities", false},
		{"GET", "/projects/project_one/capabilities/extra", false},
		{"POST", "/board/work_one/start", true},
		{"GET", "/board/work_one/start", false},
		{"POST", "/board/work_one/start/extra", false},
		{"POST", "/projects/project_one/rename", true},
		{"GET", "/projects/project_one/rename", false},
		{"POST", "/projects/project_one/rename/extra", false},
		{"POST", "/workspaces/ws_one/terminals", true},
		{"POST", "/workspaces/ws_one/terminals/terminal_one/exchange", true},
		{"POST", "/workspaces/ws_one/terminals/terminal_one/close", true},
		{"GET", "/workspaces/ws_one/terminals", false},
		{"POST", "/workspaces/ws_one/terminals/terminal_one/exec", false},
		{"GET", "/subscription", false},
		{"POST", "/subscription", false},
		{"POST", "/subscription/delete", false},
		{"GET", "/subscriptions", true},
		{"POST", "/subscriptions", true},
		{"POST", "/subscriptions/sub_one", true},
		{"POST", "/subscriptions/sub_one/delete", true},
		{"POST", "/workspaces/ws_one/subscription/load", true},
		{"POST", "/workspaces/ws_one/subscription/unload", true},
		{"POST", "/schedules/schedule_one/update", true},
		{"POST", "/schedules/schedule_one/delete", true},
		{"GET", "/schedules/schedule_one/delete", false},
		{"POST", "/schedules/schedule_one/update/extra", false},
		{"POST", "/schedules/schedule_one/unknown", false},
		{"GET", "/workspaces/ws_one/start-readiness", true},
		{"POST", "/workspaces/ws_one/start-readiness", false},
		{"GET", "/workspaces/ws_one/start-readiness/extra", false},
		{"POST", "/connection-profiles/github/delete", true},
		{"GET", "/connection-profiles/github/delete", false},
		{"POST", "/connection-profiles/github/delete/extra", false},
		{"POST", "/connection-profiles/github/unknown", false},
		{"POST", "/projects/p_one/namespace", true},
		{"GET", "/projects/p_one/namespace", false},
		{"POST", "/projects/p_one/namespace/extra", false},
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
		{"GET", "/profiles", false},
		{"GET", "/tool-profiles", false},
		{"POST", "/tool-profiles", false},
		{"POST", "/tool-profiles/general", false},
		{"DELETE", "/workspaces", false},
		{"GET", "/projects/../hermes", false},
		{"GET", "/workspaces//logs", false},
		{"GET", "/workspaces/", false},
		{"GET", "workspaces", false},
		{"HEAD", "/workspaces", false},
		{"OPTIONS", "/projects", false},
		{"POST", "/forge-connections/forge_one/issues/12/import", true},
		{"POST", "/forge-connections/forge_one/issues/invalid.id/import", false},
		{"GET", "/work-items/work_one/artifacts/artifact_one/content", true},
		{"GET", "/work-items/work_one/artifacts/invalid.id/content", false},
		{"POST", "/workspaces/ws_one/recipe-jobs/invalid.id/cancel", false},
		{"POST", "/workspaces/ws_one/terminals/invalid.id/close", false},
		{"POST", "/workspaces/ws_one/services/service_one/fetch", false},
	} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			if got := shared.DashboardBodyLimit(tt.method, tt.path) > 0; got != tt.allowed {
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

func TestFactoryWorkIngressIsExplicit(t *testing.T) {
	for _, test := range []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/work-items", true}, {"POST", "/work-items", true},
		{"GET", "/work-items/work_1", true}, {"POST", "/work-items/work_1", false},
		{"GET", "/work-items/work_1/attempts", true}, {"POST", "/work-items/work_1/attempts", false},
		{"GET", "/work-items/work_1/attempts/attempt_one/changes", true},
		{"POST", "/work-items/work_1/attempts/attempt_one/changes", false},
		{"GET", "/work-items/work_1/instruction", true}, {"POST", "/work-items/work_1/instruction", false},
		{"POST", "/work-items/work_1/continue", true}, {"GET", "/work-items/work_1/continue", false},
		{"POST", "/work-items/work_1/continue/extra", false},
		{"POST", "/work-items/work_1/run", true}, {"POST", "/work-items/work_1/cancel", true},
		{"POST", "/work-items/work_1/review", true}, {"GET", "/work-items/work_1/run", false},
		{"POST", "/work-items/work_1/exec", false}, {"GET", "/work-items/work_1/attempts/other", false},
	} {
		if got := shared.DashboardBodyLimit(test.method, test.path) > 0; got != test.allowed {
			t.Errorf("%s %s: allowed=%v", test.method, test.path, got)
		}
	}
}

func TestConversationHistoryQueryIngress(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/workspaces/ws_one/history" || r.URL.Query().Get("episode") != "task_one" || r.URL.Query().Get("after") != "20" || r.Header.Get("Authorization") != "Bearer private-operator" {
			t.Error("conversation history lost its scope, cursor or operator authority")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"page":{"records":[]}}`))
	}))
	defer upstream.Close()
	handler, err := NewInteractiveHandler(upstream.URL, upstream.URL, "private-operator")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/workspaces/ws_one/history?episode=task_one&after=20", 200},
		{"/workspaces/ws_one/history?episode=../bad&after=20", 400},
		{"/workspaces/ws_one/history?episode=task_one&episode=task_two", 400},
		{"/workspaces/ws_one/history?episode=task_one&unexpected=1", 400},
		{"/workspaces/ws_one/logs?episode=task_one", 400},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8080/ui-api"+tc.path, nil))
		if w.Code != tc.status {
			t.Errorf("%s: status %d, want %d", tc.path, w.Code, tc.status)
		}
	}
	if calls != 1 {
		t.Fatalf("unexpected upstream calls: %d", calls)
	}
}

func TestUIRouteBodyBoundaries(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	handler, err := NewInteractiveHandler(upstream.URL, upstream.URL, "private-operator")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		limit        int
	}{
		{"POST", "/forge-hosts", 4096},
		{"POST", "/forge-connections/forge_one/issues/1/import", 4096},
		{"POST", "/board/work_one/sync", 4096},
		{"POST", "/board/work_one", 100000},
		{"GET", "/board", 100000},
		{"POST", "/subscriptions/sub_one/delete", 70 << 10},
		{"POST", "/workspaces/ws_one/subscription/load", 4096},
		{"POST", "/work-items", 400000},
		{"POST", "/work-items/work_one/continue", 6 << 20},
		{"POST", "/projects/project_one/hermes/matrix/start", 32768},
		{"POST", "/connection-profiles/github/delete", 32768},
		{"POST", "/workspaces/ws_one/tasks/task_one/cancel", 70 << 10},
		{"POST", "/workspaces/ws_one/terminals/term_one/close", 70 << 10},
		{"POST", "/project-exchanges/exchange_one/exports", 100000},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			for _, over := range []int{0, 1} {
				r := httptest.NewRequest(tc.method, "http://127.0.0.1:8080/ui-api"+tc.path, strings.NewReader(strings.Repeat(" ", tc.limit+over)))
				r.Header.Set("X-Workbench-Request", "1")
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if over == 0 && w.Code != http.StatusNoContent || over != 0 && w.Code == http.StatusNoContent {
					t.Fatalf("limit + %d: status = %d", over, w.Code)
				}
			}
		})
	}
}
