package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConsoleTicketsAndOriginBoundary(t *testing.T) {
	const id = "ws_0123456789abcdef0123456789abcdef"
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer private-control-token" {
			t.Error("missing internal control authority")
		}
		if strings.HasSuffix(r.URL.Path, "/start") {
			io.WriteString(w, `{"id":"`+id+`","state":"READY"}`)
		} else {
			io.WriteString(w, `{"native":true}`)
		}
	}))
	defer upstream.Close()
	handler, err := NewInteractiveHandler(upstream.URL, upstream.URL, "private-control-token")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, host, origin, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Workbench-Request", "1")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	host := "ws-0123456789abcdef0123456789abcdef.localhost:8080"
	for _, tt := range []struct{ method, path, host, origin string }{{"GET", "/", "evil.example", ""}, {"POST", "/ui-api/workspaces", "127.0.0.1:8080", "https://evil.example"}, {"GET", "/ui-api/arbitrary", "127.0.0.1:8080", ""}, {"GET", "/", host, ""}} {
		if response := request(tt.method, tt.path, tt.host, tt.origin, `{}`, nil); response.Code < 400 {
			t.Fatal("untrusted request accepted")
		}
	}
	if calls != 0 {
		t.Fatal("untrusted request reached control plane")
	}
	start := request("POST", "/ui-api/workspaces/"+id+"/start", "127.0.0.1:8080", "http://127.0.0.1:8080", `{"idempotency_key":"start-key"}`, nil)
	if start.Code != 200 || strings.Contains(start.Body.String(), "private-control-token") {
		t.Fatal("start response unsafe")
	}
	var data struct {
		Ticket string `json:"console_ticket"`
	}
	if json.Unmarshal(start.Body.Bytes(), &data) != nil || data.Ticket == "" {
		t.Fatal("missing one-time ticket")
	}
	body := `{"ticket":"` + data.Ticket + `"}`
	connected := request("POST", "/connect", host, "http://"+host, body, nil)
	if connected.Code != 200 {
		t.Fatal("ticket not accepted")
	}
	cookies := connected.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Domain != "" || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("browser session not host-scoped")
	}
	if duplicate := request("POST", "/connect", host, "http://"+host, body, nil); duplicate.Code != 403 {
		t.Fatal("connection ticket replay accepted")
	}
	if response := request("GET", "/api/auth/session", host, "http://"+host, "", cookies[0]); response.Code != 200 || !strings.Contains(response.Body.String(), "native") {
		t.Fatal("native route failed")
	}
	if response := request("GET", "/api/auth/session", "ws-fedcba9876543210fedcba9876543210.localhost:8080", "", "", cookies[0]); response.Code != 401 {
		t.Fatal("cross-workspace cookie accepted")
	}
	if response := request("POST", "/api/mutation", host, "https://evil.example", `{}`, cookies[0]); response.Code != 403 {
		t.Fatal("cross-origin mutation accepted")
	}
}

func TestLauncherRequiresBrowserMutationHeader(t *testing.T) {
	handler, err := NewInteractiveHandler("http://controlplane:8081", "http://controlplane:8080", "control-token")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/ui-api/workspaces", strings.NewReader(`{}`))
	r.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("simple cross-site form accepted")
	}
}
