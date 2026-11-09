package firecracker

import (
	"agent-workbench/internal/shared"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
)

// The container bridge has no Docker/KVM authority. Only the host controller
// owns VM lifecycle. Preserve caller authentication, including gateway tokens.
func ServeFirecrackerBridge() error {
	socket := shared.Env("FIRECRACKER_SOCKET", "/run/agent-workbench/controller.sock")
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return errors.New("host Unix socket is unavailable; no runtime fallback")
	}
	conn.Close()
	p := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.Out.URL.Scheme = "http"
		r.Out.URL.Host = "firecracker"
	}, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}
	return shared.Serve(shared.Env("LISTEN_ADDR", ":8080"), p)
}
