package egress

import (
	"agent-workbench/internal/gateway"
	"agent-workbench/internal/shared"
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProxyRejectsUnapprovedTargetsAndMissingAuthority(t *testing.T) {
	called := false
	verify := func(context.Context, string, string) (gateway.Identity, error) {
		called = true
		return gateway.Identity{}, gateway.ErrDenied
	}
	for _, target := range []string{"example.com:443", "github.com:80", "127.0.0.1:443", "github.com.evil.test:443", "github.com:443"} {
		r := httptest.NewRequest("CONNECT", "http://"+target, nil)
		r.Host = target
		w := httptest.NewRecorder()
		Handler(verify, "", "").ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("allowed %s: %d", target, w.Code)
		}
	}
	if called {
		t.Fatal("missing authority reached verifier")
	}
	r := httptest.NewRequest("CONNECT", "http://github.com:443", nil)
	r.Host = "github.com:443"
	r.Header.Set("Proxy-Authorization", "Bearer denied")
	w := httptest.NewRecorder()
	Handler(verify, "", "").ServeHTTP(w, r)
	if !called || w.Code != 403 {
		t.Fatal("denied grant accepted")
	}
}
func TestLocalProxyInjectsGrantAndForwardsTunnel(t *testing.T) {
	got := make(chan bool, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Method == "CONNECT" && r.Host == "github.com:443" && r.Header.Get("Proxy-Authorization") == "Bearer private-grant"
		conn, buf, _ := w.(http.Hijacker).Hijack()
		defer conn.Close()
		buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		buf.Flush()
		io.Copy(conn, buf)
	}))
	defer upstream.Close()
	local := httptest.NewServer(Handler(nil, upstream.URL, "private-grant"))
	defer local.Close()
	conn, err := net.Dial("tcp", local.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprint(conn, "CONNECT github.com:443 HTTP/1.1\r\nHost: github.com:443\r\n\r\n")
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != 200 {
		t.Fatal("tunnel failed", err)
	}
	fmt.Fprint(conn, "hello")
	b := make([]byte, 5)
	if _, err = io.ReadFull(reader, b); err != nil || string(b) != "hello" {
		t.Fatal("tunnel corrupted", err)
	}
	if !<-got {
		t.Fatal("proxy authority missing or target changed")
	}
}

func TestLocalProxyPreservesPolicyDenial(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "denied", 403) }))
	defer upstream.Close()
	r := httptest.NewRequest("CONNECT", "http://github.com:443", nil)
	r.Host = "github.com:443"
	w := httptest.NewRecorder()
	Handler(nil, upstream.URL, "private-grant").ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("policy denial was not explicit: %d", w.Code)
	}
}

func TestDestinationAuthorizationUsesCurrentPolicy(t *testing.T) {
	p := &shared.ConnectionPolicy{Mode: "allowlist", Destinations: []string{"registry.npmjs.org:443"}}
	verify := func(_ context.Context, token, audience string) (gateway.Identity, error) {
		if token != "grant" || audience != "egress" {
			return gateway.Identity{}, gateway.ErrDenied
		}
		return gateway.Identity{WorkspaceID: "ws", ProjectID: "project", PrincipalID: "operator", Connections: p}, nil
	}
	r := httptest.NewRequest("CONNECT", "http://registry.npmjs.org:443", nil)
	r.Header.Set("Authorization", "Bearer grant")
	if authorize(r, verify, "registry.npmjs.org:443") != nil || authorize(r, verify, "github.com:443") == nil {
		t.Fatal("destination policy ignored")
	}
	p.Mode = "blocked"
	p.Destinations = nil
	if authorize(r, verify, "registry.npmjs.org:443") == nil {
		t.Fatal("revocation ignored")
	}
}
func TestHTTPProxyForwardsOriginCredentialsButInjectsOnlyProxyGrant(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() != "http://example.com/path" || r.Header.Get("Authorization") != "Bearer origin-credential" || r.Header.Get("Proxy-Authorization") != "Bearer workspace-grant" {
			t.Error("proxy/origin credentials confused")
		}
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	r := httptest.NewRequest("GET", "http://example.com/path", nil)
	r.Header.Set("Authorization", "Bearer origin-credential")
	r.Header.Set("Proxy-Authorization", "Bearer spoofed")
	w := httptest.NewRecorder()
	Handler(nil, upstream.URL, "workspace-grant").ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != "ok" {
		t.Fatal("HTTP proxy failed", w.Code)
	}
}
