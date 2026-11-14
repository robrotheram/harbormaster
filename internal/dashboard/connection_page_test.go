package dashboard

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestConnectionRecoveryPreservesAPIContracts(t *testing.T) {
	handler, err := NewInteractiveHandler("http://127.0.0.1:1", "http://127.0.0.1:1", "test-control-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"preview-test.localhost:5191", "ws-0123456789abcdef0123456789abcdef.localhost:5191"} {
		for _, browser := range []bool{false, true} {
			req := httptest.NewRequest("GET", "http://"+host+"/", nil)
			if browser {
				req.Header.Set("Sec-Fetch-Dest", "document")
				req.Header.Set("Accept", "text/html")
			}
			out := httptest.NewRecorder()
			handler.ServeHTTP(out, req)
			if out.Code != 401 {
				t.Fatalf("%s navigation=%v: %d", host, browser, out.Code)
			}
			body := out.Body.String()
			if strings.Contains(body, "test-control-secret") {
				t.Fatal("control credential exposed")
			}
			if browser {
				for _, want := range []string{`<html lang="en">`, `name="viewport"`, `<main`, `http://localhost:5191/app/`, `Support details`} {
					if !strings.Contains(body, want) {
						t.Fatalf("missing recovery markup %s", want)
					}
				}
				if !strings.HasPrefix(out.Header().Get("Content-Type"), "text/html") || strings.Contains(out.Header().Get("Content-Security-Policy"), "unsafe-inline") {
					t.Fatal("unsafe recovery policy")
				}
			} else if !strings.HasPrefix(out.Header().Get("Content-Type"), "application/json") {
				t.Fatal("API error became HTML")
			}
		}
		for _, path := range []string{"/connect", "/connect.js", "/_workbench/connection.css"} {
			req := httptest.NewRequest("GET", "http://"+host+path, nil)
			out := httptest.NewRecorder()
			handler.ServeHTTP(out, req)
			if out.Code != 200 || out.Header().Get("Cache-Control") != "no-store" || out.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("connection resource %s: %d %v", path, out.Code, out.Header())
			}
		}
		req := httptest.NewRequest("POST", "http://"+host+"/connect", strings.NewReader(`{"ticket":"invalid"}`))
		req.Header.Set("Origin", "http://"+host)
		req.Header.Set("X-Workbench-Request", "1")
		req.Header.Set("Accept", "text/html")
		req.Header.Set("Sec-Fetch-Dest", "document")
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		if out.Code != 403 || !strings.HasPrefix(out.Header().Get("Content-Type"), "application/json") {
			t.Fatal("ticket exchange contract changed")
		}
	}
}

func TestExpiredPreviewNavigationExplainsRecovery(t *testing.T) {
	p := newPreviewIngress(http.DefaultClient, "http://127.0.0.1:1", "secret")
	p.sessions["old"] = previewBinding{Host: "preview-test.localhost", Expiry: time.Now().Add(-time.Minute)}
	req := httptest.NewRequest("GET", "http://preview-test.localhost/", nil)
	req.Header.Set("Sec-Fetch-Dest", "iframe")
	req.Header.Set("Accept", "text/html")
	req.AddCookie(&http.Cookie{Name: "awb_preview", Value: "old"})
	out := httptest.NewRecorder()
	p.serve(out, req)
	if out.Code != 401 || !strings.Contains(out.Body.String(), `data-code="SESSION_EXPIRED"`) || !strings.Contains(out.Body.String(), "Your preview session has ended") {
		t.Fatal(out.Code, out.Body.String())
	}
	if !strings.Contains(out.Header().Get("Content-Security-Policy"), "frame-ancestors http://127.0.0.1 http://localhost") {
		t.Fatal("embedded recovery blocked")
	}
}

// Opt-in browser fixture: real ingress handlers and CSP, no live control plane.
// Used by playwright.connection.config.ts and isolated T3 review tabs.
func TestConnectionBrowserFixture(t *testing.T) {
	address := os.Getenv("WORKBENCH_CONNECTION_TEST_ADDR")
	if address == "" {
		t.Skip("browser fixture is opt-in")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/start") {
			io.WriteString(w, `{"id":"ws_0123456789abcdef0123456789abcdef","state":"READY"}`)
			return
		}
		io.WriteString(w, `{"fixture":true}`)
	}))
	defer upstream.Close()
	handler, err := NewInteractiveHandler(upstream.URL, upstream.URL, "fixture-only-secret")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_fixture/ready" && strings.HasPrefix(r.Host, "127.0.0.1:") {
			w.WriteHeader(200)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	t.Log("connection browser fixture listening on", address)
	<-ctx.Done()
}
