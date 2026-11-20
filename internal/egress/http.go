package egress

import (
	"agent-workbench/internal/gateway"
	"agent-workbench/internal/shared"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func authorize(r *http.Request, verify gateway.Verifier, target string) error {
	_, err := authorizedWorkspace(r, verify, target)
	return err
}

func authorizedWorkspace(r *http.Request, verify gateway.Verifier, target string) (string, error) {
	id, err := gateway.Authenticate(r, verify, "egress")
	if err != nil || id.Connections == nil || !id.Connections.Valid() || !id.Connections.Allows(target) {
		return "", gateway.ErrDenied
	}
	return id.WorkspaceID, nil
}
func stripHop(h http.Header) {
	for _, name := range strings.Split(h.Get("Connection"), ",") {
		h.Del(strings.TrimSpace(name))
	}
	for _, name := range []string{"Connection", "Proxy-Connection", "Proxy-Authorization", "Proxy-Authenticate", "Keep-Alive", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		h.Del(name)
	}
}
func forwardHTTP(w http.ResponseWriter, r *http.Request, verify, cached gateway.Verifier, limits *admission, upstream, token string) {
	if r.URL.Scheme != "http" || r.URL.User != nil || r.URL.Host == "" || r.Host != r.URL.Host {
		http.Error(w, "HTTP proxy request required", 403)
		return
	}
	port := r.URL.Port()
	if port == "" {
		port = "80"
	}
	target := net.JoinHostPort(r.URL.Hostname(), port)
	if !shared.ValidDestination(target) {
		http.Error(w, "invalid destination", 403)
		return
	}
	authRequest := r.Clone(r.Context())
	authRequest.Header = r.Header.Clone()
	authRequest.Header.Set("Authorization", r.Header.Get("Proxy-Authorization"))
	if upstream == "" { // only verifier context uses this header
		// Keep origin credentials separate from workspace authority.
		workspace, err := authorizedWorkspace(authRequest, cached, target)
		if err != nil {
			http.Error(w, "connection denied", 403)
			return
		}
		release, ok := limits.acquire(workspace)
		if !ok {
			http.Error(w, "workspace connection limit reached", 429)
			return
		}
		defer release()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	out := r.Clone(ctx)
	out.RequestURI = ""
	out.Header = r.Header.Clone()
	stripHop(out.Header)
	transport, _ := (gateway.NetworkPolicy{AllowedCIDRs: []string{"0.0.0.0/0", "::/0"}}).Transport(nil)
	if upstream != "" {
		u, err := url.Parse(upstream)
		if err != nil || u.Scheme != "http" {
			http.Error(w, "gateway unavailable", 503)
			return
		}
		transport = &http.Transport{Proxy: http.ProxyURL(u), ResponseHeaderTimeout: 30 * time.Second}
		out.Header.Set("Proxy-Authorization", "Bearer "+token)
	}
	defer transport.CloseIdleConnections()
	done := make(chan struct{})
	defer close(done)
	if upstream == "" {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					check, c := context.WithTimeout(ctx, 5*time.Second)
					err := authorize(authRequest.WithContext(check), verify, target)
					c()
					if err != nil {
						cancel()
						return
					}
				}
			}
		}()
	}
	out.Body = http.MaxBytesReader(w, out.Body, 32<<20)
	response, err := transport.RoundTrip(out)
	if err != nil {
		http.Error(w, "gateway unavailable", 503)
		return
	}
	defer response.Body.Close()
	stripHop(response.Header)
	for k, values := range response.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(response.StatusCode)
	io.Copy(w, response.Body)
}
