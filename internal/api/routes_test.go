package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-workbench/internal/shared"
)

func TestSharedHumanRouteParity(t *testing.T) {
	s, _ := setup(t)
	s.cfg.SharedWorkspaceID = "ws_shared"
	token, err := shared.IssueHumanAccess(s.cfg.Token, shared.HumanIdentity{ID: "user_alice", Email: "alice@example.com"}, s.now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	// Observe the authorization decision before ServeMux applies its own method
	// and path rules or the handlers check project membership.
	handler := s.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/v1/projects", true},
		{"DELETE", "/v1/projects", true},
		{"GET", "/v1/projects/", false},
		{"HEAD", "/v1/projects/project_one/members/", true},
		{"POST", "/v1/projects//rename", true},
		{"PUT", "/v1/projects/invalid.id/capabilities", true},
		{"POST", "/v1/projects/project_one/hermes", false},
		{"POST", "/v1/projects/project_one/agent-access", false},
		{"POST", "/v1/project-invitations", true},
		{"HEAD", "/v1/templates", true},
		{"GET", "/v1/board", true},
		{"HEAD", "/v1/board", false},
		{"POST", "/v1/board/invalid.id/", true},
		{"POST", "/v1/board/work_one/start", false},
		{"GET", "/v1/work-items", true},
		{"POST", "/v1/work-items", false},
		{"GET", "/v1/work-items/invalid.id/", true},
		{"GET", "/v1/work-items/work_one/attempts", true},
		{"GET", "/v1/work-items/work_one/extra/attempts", true},
		{"GET", "/v1/work-items/work_one/attempts/", false},
		{"GET", "/v1/work-items/work_one/instruction", false},
		{"HEAD", "/v1/work-items/work_one", false},
		{"GET", "/v1/subscriptions", true},
		{"POST", "/v1/subscriptions", true},
		{"HEAD", "/v1/subscriptions", false},
		{"POST", "/v1/subscriptions/sub_one/", true},
		{"POST", "/v1/subscriptions/sub_one/delete/", true},
		{"POST", "/v1/subscriptions/invalid.id/delete", false},
		{"POST", "/v1/workspaces/ws_shared/subscription/load", false},
		{"GET", "/v1/workspaces/ws_shared/logs", false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			want := http.StatusConflict
			if tc.allowed {
				want = http.StatusNoContent
			}
			if w.Code != want {
				t.Fatalf("status = %d, want %d", w.Code, want)
			}
		})
	}
}

func TestAPIRouteBodyBoundaries(t *testing.T) {
	s, _ := setup(t)
	for _, tc := range []struct {
		path, body, acceptedCode string
		limit                    int
	}{
		{"/v1/workspaces/missing/start", `{"idempotency_key":"start-workspace"}`, "NOT_FOUND_OR_FORBIDDEN", 1024},
		{"/v1/projects", `{"name":"Demo","idempotency_key":"test-limit"}`, "", 1024},
		{"/v1/board/unknown/sync", `{"project_id":"missing"}`, "NOT_FOUND_OR_FORBIDDEN", 100000},
		{"/v1/work-items", `{"project_id":"missing"}`, "NOT_FOUND_OR_FORBIDDEN", 400000},
		{"/v1/work-items/unknown/continue", `{"project_id":"missing"}`, "NOT_FOUND_OR_FORBIDDEN", 6 << 20},
	} {
		t.Run(tc.path, func(t *testing.T) {
			for _, over := range []int{0, 1} {
				body := tc.body + strings.Repeat(" ", tc.limit-len(tc.body)+over)
				w := domainRequest(t, s, s.cfg.Token, http.MethodPost, tc.path, body)
				want := tc.acceptedCode
				if over != 0 {
					want = "INVALID_REQUEST"
				}
				if want == "" {
					if w.Code != http.StatusCreated {
						t.Fatalf("at limit: %d %s", w.Code, w.Body)
					}
				} else if !strings.Contains(w.Body.String(), `"code":"`+want+`"`) {
					t.Fatalf("limit + %d: %d %s, want %s", over, w.Code, w.Body, want)
				}
			}
		})
	}
}
