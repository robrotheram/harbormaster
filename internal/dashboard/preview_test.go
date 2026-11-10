package dashboard

import (
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
