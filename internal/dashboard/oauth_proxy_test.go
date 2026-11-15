package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOAuthProxyValidatesSessionRatherThanForwardedIdentity(t *testing.T) {
	for _, status := range []int{200, 202, 401, 403, 302, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/oauth2/auth" || r.Method != "GET" {
					t.Errorf("unexpected auth request: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Cookie") != "session=test" {
					t.Error("session was not forwarded")
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("X-Auth-Request-User") != "" {
					t.Error("caller identity was trusted")
				}
				if r.Header.Get("X-Forwarded-Proto") != "https" || r.Header.Get("X-Forwarded-Host") != "reference.example" {
					t.Error("noncanonical origin")
				}
				w.Header().Set("X-Auth-Request-User", "verified-operator")
				w.Header().Set("Location", "https://untrusted.invalid")
				w.WriteHeader(status)
			}))
			defer proxy.Close()
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Context().Value(authenticatedOriginKey{}) != "https://reference.example" {
					t.Error("missing authenticated origin")
				}
				w.WriteHeader(204)
			})
			handler, err := NewOAuthProxyHandler(next, OAuthProxyConfig{"https://reference.example", proxy.URL + "/oauth2/auth", "https://signin.example"})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", "https://reference.example/ui-api/workspaces", strings.NewReader(`{}`))
			r.Header.Set("Cookie", "session=test")
			r.Header.Set("Origin", "https://reference.example")
			r.Header.Set("X-Auth-Request-User", "forged")
			r.Header.Set("Authorization", "Bearer do-not-forward")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			want := 503
			if status == 200 || status == 202 {
				want = 204
			} else if status == 401 || status == 403 {
				want = 401
			}
			if w.Code != want || (calls == 1) != (want == 204) {
				t.Fatalf("status=%d calls=%d; want %d", w.Code, calls, want)
			}
		})
	}
}

func TestOAuthProxySessionOriginAndLogout(t *testing.T) {
	authenticated := true
	identity := true
	calls := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !authenticated {
			w.WriteHeader(401)
			return
		}
		if identity {
			w.Header().Set("X-Auth-Request-User", "operator")
			w.Header().Set("X-Auth-Request-Email", "operator@example.test")
		}
		w.WriteHeader(202)
	}))
	defer proxy.Close()
	h, err := NewOAuthProxyHandler(http.NotFoundHandler(), OAuthProxyConfig{"https://reference.example", proxy.URL + "/oauth2/auth", "https://signin.example"})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://reference.example"+path, nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		r.Header.Set("X-Workbench-Request", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := request("GET", "/auth/session", "")
	var data struct {
		Status string `json:"status"`
		User   struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil || data.Status != "authenticated" || data.User.ID != "operator" {
		t.Fatalf("invalid session %s", w.Body.String())
	}
	before := calls
	if w = request("POST", "/ui-api/workspaces", "https://foreign.example"); w.Code != 403 || calls != before {
		t.Fatal("foreign origin was not rejected before auth")
	}
	if w = request("POST", "/auth/logout", ""); w.Code != 403 {
		t.Fatal("logout accepted without Origin")
	}
	if w = request("POST", "/auth/logout", "https://reference.example"); w.Code != 200 || !strings.Contains(w.Body.String(), "https://signin.example/oauth2/sign_out") {
		t.Fatal("logout did not use configured authority")
	}
	identity = false
	if w = request("GET", "/auth/session", ""); w.Code != 503 {
		t.Fatal("accepted identity-free auth response")
	}
	authenticated = false
	if w = request("GET", "/auth/session", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "anonymous") {
		t.Fatal("expired session not cleared")
	}
	before = calls
	r := httptest.NewRequest("GET", "https://foreign.example/app/", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || calls != before {
		t.Fatal("foreign host was accepted")
	}
	if w = request("GET", "/healthz", ""); w.Code != 200 || calls != before {
		t.Fatal("health depends on external auth")
	}
	proxy.Close()
	if w = request("GET", "/app/", ""); w.Code != 503 {
		t.Fatal("auth outage did not fail closed")
	}
}

func TestOAuthProxyRequiresCompleteConfiguration(t *testing.T) {
	for _, cfg := range []OAuthProxyConfig{
		{}, {"https://reference.example", "", "https://signin.example"},
		{"http://reference.example", "https://auth.example/oauth2/auth", "https://signin.example"},
		{"https://reference.example/path", "https://auth.example/oauth2/auth", "https://signin.example"},
		{"https://reference.example", "https://user:password@auth.example/oauth2/auth", "https://signin.example"},
	} {
		if _, err := NewOAuthProxyHandler(http.NotFoundHandler(), cfg); err == nil {
			t.Fatalf("accepted invalid config: %+v", cfg)
		}
	}
}
