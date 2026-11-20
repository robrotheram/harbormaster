// Package egress implements the approved HTTPS CONNECT path. Guest DNS remains
// restricted: destination resolution happens in the trusted gateway.
package egress

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"agent-workbench/internal/gateway"
	"agent-workbench/internal/shared"
)

func Handler(verify gateway.Verifier, upstream, token string) http.Handler {
	limits := &admission{grants: make(map[[32]byte]cachedGrant), active: make(map[string]int)}
	cached := limits.verifier(verify)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/healthz" {
			w.Write([]byte("ok"))
			return
		}
		if r.Method != "CONNECT" {
			forwardHTTP(w, r, verify, cached, limits, upstream, token)
			return
		}
		if !shared.ValidDestination(r.Host) {
			http.Error(w, "destination not approved", 403)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var remote net.Conn
		var reader *bufio.Reader
		var err error
		failureStatus := http.StatusServiceUnavailable
		if upstream != "" {
			u, e := url.Parse(upstream)
			if e != nil || u.Scheme != "http" {
				http.Error(w, "gateway unavailable", 503)
				return
			}
			remote, err = (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", u.Host)
			if err == nil {
				remote.SetDeadline(time.Now().Add(10 * time.Second))
				_, err = fmt.Fprintf(remote, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Bearer %s\r\n\r\n", r.Host, r.Host, token)
				reader = bufio.NewReader(remote)
				if err == nil {
					var response *http.Response
					response, err = http.ReadResponse(reader, r)
					if err == nil && response.StatusCode != 200 {
						if response.StatusCode == http.StatusForbidden {
							failureStatus = http.StatusForbidden
						}
						err = gateway.ErrDenied
					}
				}
			}
		} else {
			r.Header.Set("Authorization", r.Header.Get("Proxy-Authorization"))
			workspace, authErr := authorizedWorkspace(r, cached, r.Host)
			if authErr != nil {
				http.Error(w, "Connection denied by workspace policy", 403)
				return
			}
			release, ok := limits.acquire(workspace)
			if !ok {
				http.Error(w, "workspace connection limit reached", 429)
				return
			}
			defer release()
			transport, _ := (gateway.NetworkPolicy{AllowedCIDRs: []string{"0.0.0.0/0", "::/0"}}).Transport(nil)
			remote, err = transport.DialContext(ctx, "tcp", r.Host)
		}
		if err != nil {
			if remote != nil {
				remote.Close()
			}
			if failureStatus == http.StatusForbidden {
				http.Error(w, "Connection denied by workspace policy", failureStatus)
			} else {
				http.Error(w, "approved gateway unavailable", failureStatus)
			}
			return
		}
		defer remote.Close()
		remote.SetDeadline(time.Time{})
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "tunnel unsupported", 500)
			return
		}
		client, buffer, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		buffer.Flush()
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
					case <-ticker.C:
						check, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						e := authorize(r.WithContext(check), verify, r.Host)
						cancel()
						if e != nil {
							remote.Close()
							client.Close()
							return
						}
					}
				}
			}()
		}
		if reader == nil {
			reader = bufio.NewReader(remote)
		}
		go func() { io.Copy(remote, buffer); remote.Close() }()
		io.Copy(client, reader)
	})
}

// Environment sends external requests through the local proxy while keeping
// trusted, runtime-configured platform endpoints on their direct network path.
func Environment(env []string, endpoints ...string) []string {
	bypass := "127.0.0.1,localhost,inference-gateway,credential-broker"
	for _, endpoint := range endpoints {
		u, err := url.Parse(endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			continue
		}
		host := u.Hostname()
		if host != "" && !strings.ContainsAny(host, ",* \t\r\n") {
			bypass += "," + host
		}
	}
	return append(env, "HTTP_PROXY=http://127.0.0.1:8099", "http_proxy=http://127.0.0.1:8099", "HTTPS_PROXY=http://127.0.0.1:8099", "https_proxy=http://127.0.0.1:8099", "NO_PROXY="+bypass, "no_proxy="+bypass)
}
