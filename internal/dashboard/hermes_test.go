package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHermesUIRouteRequiresLocalMutationAuthority(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/projects/project_one/hermes" || r.Header.Get("Authorization") != "Bearer operator" {
			t.Error("wrong authorized setup route")
		}
		w.Write([]byte(`{"saved":false}`))
	}))
	defer upstream.Close()
	h, err := NewInteractiveHandler(upstream.URL, upstream.URL, "operator")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		method, origin, marker string
		want                   int
	}{
		{"GET", "", "", 200},
		{"POST", "http://127.0.0.1:8080", "1", 200},
		{"POST", "https://foreign.example", "1", 403},
		{"POST", "http://127.0.0.1:8080", "", 403},
	} {
		r := httptest.NewRequest(tt.method, "http://127.0.0.1:8080/ui-api/projects/project_one/hermes", strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", tt.origin)
		r.Header.Set("X-Workbench-Request", tt.marker)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tt.want {
			t.Fatal(tt.method, tt.origin, w.Code, w.Body.String())
		}
	}
	if calls != 2 {
		t.Fatal("untrusted configuration reached control plane", calls)
	}
}
