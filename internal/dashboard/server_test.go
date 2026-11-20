package dashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func handler(t *testing.T, upstream string) http.Handler {
	t.Helper()
	h, err := NewHandler(upstream, "")
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func request(h http.Handler, method, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Authorization", "Bearer must-not-forward")
	r.Header.Set("Cookie", "secret=must-not-forward")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestSnapshotProxyAndAllowlist(t *testing.T) {
	var calls atomic.Int32
	payload := `{"schema_version":1,"mode":"development","workspaces":{"items":[]}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.RequestURI() != snapshotPath {
			t.Errorf("unexpected upstream route: %s %s", r.Method, r.URL.RequestURI())
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("forwarded caller credentials")
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("missing JSON accept")
		}
		w.Header().Set("Set-Cookie", "must-not-forward=yes")
		_, _ = io.WriteString(w, payload)
	}))
	defer upstream.Close()
	h := handler(t, upstream.URL)
	w := request(h, "GET", snapshotPath)
	if w.Code != 200 || w.Body.String() != payload {
		t.Fatalf("snapshot: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Set-Cookie") != "" {
		t.Error("forwarded upstream cookie")
	}
	for _, path := range []string{"/dashboard-api/v1/snapshot/", "/dashboard-api/v1/snapshot?", "/dashboard-api/v1/%73napshot", "/dashboard-api/v1/snapshot?url=http://example.com", "/dashboard-api/v1/snapshot/extra", "/v1/workspaces", "/dashboard-api/v1/other", "/web/index.html", "/../healthz"} {
		if w := request(h, "GET", path); w.Code != 404 {
			t.Errorf("%s: got %d", path, w.Code)
		}
	}
	for _, method := range []string{"HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT"} {
		for _, path := range []string{snapshotPath, "/", "/healthz", "/v1/workspaces"} {
			w := request(h, method, path)
			if w.Code != 405 || w.Header().Get("Allow") != "GET" {
				t.Errorf("%s %s: got %d", method, path, w.Code)
			}
		}
	}
	if calls.Load() != 1 {
		t.Errorf("unexpected upstream calls: %d", calls.Load())
	}
}

func TestUpstreamFailuresAreSanitized(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"error", 500, "private upstream detail"}, {"unauthorized", 401, "private token detail"},
		{"html", 200, "<html>private upstream detail</html>"}, {"invalid JSON", 200, `{"private":"detail"`},
		{"null", 200, "null"}, {"array", 200, "[]"}, {"oversize", 200, `{"data":"` + strings.Repeat("x", maxSnapshotBytes) + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			w := request(handler(t, upstream.URL), "GET", snapshotPath)
			if w.Code != 503 || !strings.Contains(w.Body.String(), `"code":"SNAPSHOT_UNAVAILABLE"`) || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("unsafe failure: %d %s", w.Code, w.Body.String())
			}
		})
	}
	t.Run("unreachable", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		upstream.Close()
		if w := request(handler(t, upstream.URL), "GET", snapshotPath); w.Code != 503 {
			t.Errorf("got %d", w.Code)
		}
	})
	t.Run("redirect", func(t *testing.T) {
		var followed atomic.Bool
		destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Store(true) }))
		defer destination.Close()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
		defer upstream.Close()
		if w := request(handler(t, upstream.URL), "GET", snapshotPath); w.Code != 503 {
			t.Errorf("got %d", w.Code)
		}
		if followed.Load() {
			t.Error("followed upstream redirect")
		}
	})
}

func TestReactEntrypointRedirectsAndHealth(t *testing.T) {
	h := handler(t, "http://controlplane:8081")
	for _, path := range []string{"/", "/workspaces", "/getting-started"} {
		w := request(h, "GET", path)
		if w.Code != http.StatusTemporaryRedirect || !strings.HasPrefix(w.Header().Get("Location"), "/app/") {
			t.Errorf("%s does not lead to React", path)
		}
	}
	if w := request(h, "GET", "/healthz"); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Fatal("health unavailable")
	}
}

func TestOriginValidation(t *testing.T) {
	for _, origin := range []string{"", "://bad", "file:///tmp/snapshot", "http://user:password@controlplane", "http://controlplane/path", "http://controlplane?token=x", "http://controlplane#x"} {
		if _, err := NewHandler(origin, ""); err == nil {
			t.Errorf("accepted invalid origin")
		}
	}
}
