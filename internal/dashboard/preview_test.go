package dashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPreviewIngressBudgetsMethodsAndSessionExpiry(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("x", (3<<20)+1))) }))
	defer upstream.Close()
	p := newPreviewIngress(upstream.Client(), upstream.URL, "root-secret")
	binding := previewBinding{Workspace: "ws_one", Service: "service_one", Host: "preview-one.localhost", Expiry: time.Now().Add(time.Hour)}
	p.sessions["session"] = binding
	invoke := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Host = binding.Host
		req.AddCookie(&http.Cookie{Name: "awb_preview", Value: "session"})
		out := httptest.NewRecorder()
		p.serve(out, req)
		return out
	}
	if out := invoke("GET", "/"); out.Code != 503 {
		t.Fatalf("response budget: %d", out.Code)
	}
	if out := invoke("POST", "/index.html"); out.Code != 405 {
		t.Fatalf("method: %d", out.Code)
	}
	if out := invoke("POST", "/connect"); out.Code != 403 {
		t.Fatalf("CSRF: %d", out.Code)
	}
	if out := invoke("GET", "/../secret"); out.Code != 400 {
		t.Fatalf("path: %d", out.Code)
	}
	binding.Expiry = time.Now().Add(-time.Second)
	p.sessions["session"] = binding
	if out := invoke("GET", "/"); out.Code != 401 {
		t.Fatalf("expiry: %d", out.Code)
	}
}

func TestPreviewConnectExpiredTicketsAndBodyBudget(t *testing.T) {
	p := newPreviewIngress(http.DefaultClient, "http://unused", "secret")
	host := "preview-one.localhost"
	p.tickets["expired"] = previewBinding{Workspace: "ws_one", Service: "service_one", Host: host, Expiry: time.Now().Add(-time.Second)}
	for _, tc := range []struct {
		body string
		want int
	}{{`{"ticket":"expired"}`, 403}, {`{"ticket":"` + strings.Repeat("x", 2048) + `"}`, 400}} {
		req := httptest.NewRequest("POST", "/connect", strings.NewReader(tc.body))
		req.Host = host
		req.Header.Set("Origin", "http://"+host)
		req.Header.Set("X-Workbench-Request", "1")
		out := httptest.NewRecorder()
		p.serve(out, req)
		if out.Code != tc.want {
			t.Fatalf("connect budget/expiry %d %s", out.Code, out.Body)
		}
	}
	p.sessions["session"] = previewBinding{Workspace: "ws_one", Service: "service_one", Host: host, Expiry: time.Now().Add(time.Hour)}
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = host
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.AddCookie(&http.Cookie{Name: "awb_preview", Value: "session"})
	out := httptest.NewRecorder()
	p.serve(out, req)
	if out.Code != 403 {
		t.Fatal("cross-origin same-site resource admitted")
	}
}

func TestApplicationPreviewForwardsHTTPWithoutManagementCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspaces/ws_app/application-proxy/service_app/api" || r.Header.Get("Authorization") != "Bearer operator-only" {
			t.Error("incorrect management authority")
		}
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.RawQuery != "x=1" || string(body) != "hello" {
			t.Error("lost application request")
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(201)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	p := newPreviewIngress(upstream.Client(), upstream.URL, "operator-only")
	p.sessions["session"] = previewBinding{Application: true, Workspace: "ws_app", Service: "service_app", Host: "preview-one.localhost", Expiry: time.Now().Add(time.Hour)}
	for _, origin := range []string{"http://foreign.localhost", "http://preview-one.localhost"} {
		req := httptest.NewRequest("POST", "http://preview-one.localhost/api?x=1", strings.NewReader("hello"))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "text/plain")
		req.AddCookie(&http.Cookie{Name: "awb_preview", Value: "session"})
		out := httptest.NewRecorder()
		p.serve(out, req)
		if origin == "http://foreign.localhost" {
			if out.Code != 403 {
				t.Fatal("foreign mutation accepted", out.Code)
			}
		} else if out.Code != 201 || out.Body.String() != "ok" {
			t.Fatal(out.Code, out.Body.String())
		}
	}
}

func TestEmbeddedPreviewUsesPartitionedSessionAndBoundInspector(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Write([]byte("<h1>Application</h1>"))
	}))
	defer upstream.Close()
	p := newPreviewIngress(upstream.Client(), upstream.URL, "operator")
	host := "preview-test.localhost:8080"
	p.tickets["ticket"] = previewBinding{Embedded: true, Application: true, ParentOrigin: "http://127.0.0.1:8080", Workspace: "ws_one", Service: "service_one", Host: host, Expiry: time.Now().Add(time.Minute), ServiceExpiry: time.Now().Add(time.Hour)}
	req := httptest.NewRequest("POST", "http://"+host+"/connect", strings.NewReader(`{"ticket":"ticket"}`))
	req.Header.Set("Origin", "http://"+host)
	req.Header.Set("X-Workbench-Request", "1")
	out := httptest.NewRecorder()
	p.serve(out, req)
	cookies := out.Result().Cookies()
	if out.Code != 200 || len(cookies) != 1 || !cookies[0].Secure || !cookies[0].Partitioned || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteNoneMode {
		t.Fatal("embedded session cookie", out.Header())
	}
	req = httptest.NewRequest("GET", "http://"+host+"/", nil)
	req.AddCookie(cookies[0])
	out = httptest.NewRecorder()
	p.serve(out, req)
	if out.Code != 200 || !strings.Contains(out.Body.String(), `const origin="http://127.0.0.1:8080"`) || !strings.Contains(out.Body.String(), "e.source!==parent") {
		t.Fatal("missing bound inspector", out.Body.String())
	}
	policies := out.Header().Values("Content-Security-Policy")
	if len(policies) != 1 || !strings.Contains(policies[0], "script-src 'self' 'unsafe-inline'") {
		t.Fatal("conflicting preview policies", policies)
	}
	if out.Header().Get("X-Frame-Options") != "" || !strings.Contains(out.Header().Get("Content-Security-Policy"), "frame-ancestors http://127.0.0.1:8080 http://localhost:8080") {
		t.Fatal("embedding policy", out.Header())
	}
}
