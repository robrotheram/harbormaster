package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplicationAdmissionBeforeWorkspaceStart(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "preview-admission")
	path := "/v1/workspaces/" + ws.ID + "/applications"
	for _, body := range []string{`{"application":{"directory":".","command":"python3 -m http.server","port":3773},"ttl_seconds":3600,"idempotency_key":"invalid"}`, `{"application":{"directory":"../","command":"python3 -m http.server","port":4173},"ttl_seconds":3600,"idempotency_key":"invalid"}`} {
		if got := request(t, s, "POST", path, body); got.Code != 400 {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	if rt.starts != 0 {
		t.Fatal("invalid application started worker")
	}
	r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("unauthorized application admitted", w.Code)
	}
	if got := request(t, s, "GET", path, ""); got.Code != 200 || !strings.Contains(got.Body.String(), "workspace_stopped") {
		t.Fatal(got.Code, got.Body.String())
	}
	if rt.starts != 0 {
		t.Fatal("read woke worker")
	}
}
