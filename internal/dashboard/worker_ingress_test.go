package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostedSharedWorkerRequiresTicketAndSecureCookie(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"id":"ws_shared","state":"READY"}`)) }))
	defer up.Close()
	interactive, err := NewInteractiveHandler(up.URL, up.URL, "control")
	if err != nil {
		t.Fatal(err)
	}
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Auth-Request-User", "alice")
		w.WriteHeader(202)
	}))
	defer auth.Close()
	h, err := NewOAuthProxyHandler(interactive, OAuthProxyConfig{"https://reference.example", auth.URL + "/oauth2/auth", "https://login.example"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, url, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, url, strings.NewReader(body))
		r.Header.Set("X-Workbench-Request", "1")
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	start := call("POST", "https://reference.example/ui-api/workspaces/ws_shared/start", `{"idempotency_key":"start"}`, nil)
	var result struct {
		Ticket string `json:"console_ticket"`
	}
	json.Unmarshal(start.Body.Bytes(), &result)
	if result.Ticket == "" {
		t.Fatal(start.Code, start.Body)
	}
	if w := call("GET", "https://ws-shared.reference.example/", "", nil); w.Code != 401 {
		t.Fatal("unauthenticated worker access", w.Code)
	}
	if w := call("GET", "https://ws-shared.reference.example/ui-api/subscription", "", nil); w.Code != 401 {
		t.Fatal("worker origin can manage subscriptions", w.Code)
	}
	connected := call("POST", "https://ws-shared.reference.example/connect", `{"ticket":"`+result.Ticket+`"}`, nil)
	if connected.Code != 200 {
		t.Fatal(connected.Code, connected.Body)
	}
	cookies := connected.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || cookies[0].Domain != "" || !cookies[0].HttpOnly {
		t.Fatal("unsafe session cookie")
	}
	if w := call("GET", "https://ws-shared.reference.example/", "", cookies[0]); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := call("POST", "https://ws-shared.reference.example/connect", `{"ticket":"`+result.Ticket+`"}`, nil); w.Code != 403 {
		t.Fatal("replayed ticket accepted")
	}
}
