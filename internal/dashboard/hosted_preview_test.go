package dashboard

import (
	"agent-workbench/internal/shared"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostedPreviewConnection(t *testing.T) {
	var revoked atomic.Bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		human, err := shared.VerifyHumanAccess("control", strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), time.Now())
		if err != nil || human.Email != "alice@example.com" {
			t.Errorf("preview lost principal: %+v %v", human, err)
			w.WriteHeader(401)
			return
		}
		if revoked.Load() {
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case "/v1/workspaces/ws_shared":
			w.Write([]byte(`{"project_id":"project_one"}`))
		case "/v1/workspaces/ws_shared/services/service_one":
			json.NewEncoder(w).Encode(map[string]any{"kind": "application", "state": "active", "expires_at": time.Now().Add(time.Hour).Unix()})
		case "/v1/workspaces/ws_shared/application-proxy/service_one/":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<h1>Preview</h1>"))
		default:
			t.Errorf("unexpected upstream request: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	interactive, err := NewInteractiveHandler(up.URL, up.URL, "control")
	if err != nil {
		t.Fatal(err)
	}
	authCalls := 0
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCalls++
		w.Header().Set("X-Auth-Request-User", "alice")
		w.Header().Set("X-Auth-Request-Email", "alice@example.com")
		w.WriteHeader(202)
	}))
	defer auth.Close()
	h, err := NewOAuthProxyHandler(interactive, OAuthProxyConfig{"https://reference.example", auth.URL + "/oauth2/auth", "https://login.example", []string{"alice@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, address, origin, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, address, strings.NewReader(body))
		// Simulate TLS termination: only authenticated ingress context is trusted.
		r.TLS = nil
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Workbench-Request", "1")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-Host", "attacker.example")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<main>Workbench</main>"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FRONTEND_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "preview-review.js"), []byte("/* review bridge */"), 0600); err != nil {
		t.Fatal(err)
	}
	shell := call("GET", "https://reference.example/app/", "", "", nil)
	if shell.Code != 200 || !strings.Contains(shell.Header().Get("Content-Security-Policy"), "frame-src https://*.reference.example;") {
		t.Fatal("dashboard blocks hosted preview frames", shell.Code, shell.Header())
	}
	shellPolicy := shell.Header().Get("Content-Security-Policy")
	if !strings.Contains(shellPolicy, "script-src 'self' 'wasm-unsafe-eval';") || strings.Contains(shellPolicy, "'unsafe-eval'") {
		t.Fatal("unexpected dashboard script policy", shellPolicy)
	}
	issued := call("POST", "https://reference.example/ui-api/workspaces/ws_shared/services/service_one/connect", "https://reference.example", `{"embedded":true}`, nil)
	var result struct {
		URL string `json:"preview_url"`
	}
	if err := json.Unmarshal(issued.Body.Bytes(), &result); err != nil || issued.Code != 200 {
		t.Fatal(issued.Code, issued.Body.String())
	}
	u, err := url.Parse(result.URL)
	if err != nil || u.Scheme != "https" || publicWorkerLabel(u.Host, "reference.example") == "" || !strings.HasPrefix(u.Host, "preview-") {
		t.Fatal("invalid hosted preview URL", result.URL)
	}
	origin := "https://" + u.Host
	ticket := strings.TrimPrefix(u.Fragment, "ticket=")
	body := `{"ticket":"` + ticket + `"}`
	callsBefore := authCalls
	landing := call("GET", origin+"/connect", "", "", nil)
	if landing.Code != 200 || !strings.Contains(landing.Header().Get("Content-Security-Policy"), "frame-ancestors https://reference.example") || !strings.Contains(landing.Body.String(), "https://reference.example/app/") {
		t.Fatal("invalid hosted landing", landing.Code, landing.Header(), landing.Body.String())
	}
	if w := call("POST", origin+"/connect", "https://foreign.example", body, nil); w.Code != 403 {
		t.Fatal("foreign origin accepted", w.Code)
	}
	connected := call("POST", origin+"/connect", origin, body, nil)
	cookies := connected.Result().Cookies()
	if connected.Code != 200 || len(cookies) != 1 || cookies[0].Name != "__Host-awb_preview" || !cookies[0].Secure || !cookies[0].Partitioned || cookies[0].Domain != "" {
		t.Fatal("hosted exchange failed", connected.Code, connected.Body.String(), cookies)
	}
	if w := call("POST", origin+"/connect", origin, body, nil); w.Code != 403 {
		t.Fatal("ticket replay accepted", w.Code)
	}
	live := call("GET", origin+"/", "", "", cookies[0])
	if live.Code != 200 || !strings.Contains(live.Body.String(), `const origin="https://reference.example"`) || !strings.Contains(live.Header().Get("Content-Security-Policy"), "wss://"+u.Host) {
		t.Fatal("hosted application failed", live.Code, live.Header(), live.Body.String())
	}
	if strings.Contains(live.Header().Get("Content-Security-Policy"), "wasm-unsafe-eval") {
		t.Fatal("preview inherited dashboard WebAssembly permission")
	}
	if !strings.Contains(live.Body.String(), `src="/.workbench/preview-review.js" data-parent-origin="https://reference.example"`) {
		t.Fatal("embedded preview did not load the review bridge")
	}
	if guest := call("GET", origin+"/.workbench/preview-review.js", "", "", nil); guest.Code != 401 {
		t.Fatal("preview review bridge served without a connection", guest.Code)
	}
	if asset := call("GET", origin+"/.workbench/preview-review.js", "", "", cookies[0]); asset.Code != 200 || asset.Body.String() != "/* review bridge */" {
		t.Fatal("preview review bridge was not served", asset.Code)
	}
	if w := call("GET", origin+"/ui-api/subscriptions", "", "", nil); w.Code != 401 {
		t.Fatal("preview exposed management API", w.Code)
	}
	if authCalls != callsBefore {
		t.Fatal("preview requests reached operator authentication")
	}
	revoked.Store(true)
	if w := call("GET", origin+"/", "", "", cookies[0]); w.Code != 403 {
		t.Fatal("revoked preview stayed accessible", w.Code)
	}
	for _, host := range []string{"preview-bad.reference.example", "preview-0123456789abcdef0123456789abcdef.evil.example", "nested.preview-0123456789abcdef0123456789abcdef.reference.example"} {
		if w := call("GET", "https://"+host+"/connect", "", "", nil); w.Code != 403 {
			t.Fatal("unrecognized host accepted", host, w.Code)
		}
	}
}

func TestDuplicateHostCookiesAreRejected(t *testing.T) {
	req := httptest.NewRequest("GET", "https://preview.example/", nil)
	req.Header.Set("Cookie", "__Host-awb_preview=first; __Host-awb_preview=second")
	if _, err := uniqueCookie(req, "__Host-awb_preview"); err == nil {
		t.Fatal("duplicate HTTPS session cookie accepted")
	}
}
