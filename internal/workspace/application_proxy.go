package workspace

import (
	"agent-workbench/internal/shared"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Only a registered, live process can be proxied. The client supplies a service
// identity and request path, never a host or port. HTTP and WebSockets share this
// path, including revocation when the process group is stopped or expires.
func (m *Manager) proxyApplication(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("service")
	if !shared.ValidID(id) {
		shared.Error(w, 404, "SERVICE_NOT_FOUND")
		return
	}
	m.mu.Lock()
	s, err := m.service(id)
	var raw string
	if err == nil {
		err = m.db.QueryRow(`SELECT configuration FROM workspace_applications WHERE service_id=?`, id).Scan(&raw)
	}
	m.mu.Unlock()
	if err != nil {
		shared.Error(w, 404, "SERVICE_NOT_FOUND")
		return
	}
	if s.State != "active" {
		shared.Error(w, 410, "SERVICE_REVOKED")
		return
	}
	var app shared.ApplicationConfig
	if json.Unmarshal([]byte(raw), &app) != nil || !app.Valid() {
		shared.Error(w, 503, "INVALID_APPLICATION")
		return
	}
	target := net.JoinHostPort("127.0.0.1", strconv.Itoa(app.Port))
	if app.Kind == "forward" {
		port, ok := forwardTarget(app)
		if !ok {
			shared.Error(w, 410, "APPLICATION_PORT_CLOSED")
			return
		}
		target = port.Address
	}
	transport := &http.Transport{Proxy: nil, ResponseHeaderTimeout: 15 * time.Second, IdleConnTimeout: 10 * time.Second}
	defer transport.CloseIdleConnections()
	proxy := httputil.ReverseProxy{Transport: transport, FlushInterval: -1, Rewrite: func(p *httputil.ProxyRequest) {
		p.Out.URL.Scheme = "http"
		p.Out.URL.Host = target
		p.Out.URL.Path = "/" + r.PathValue("path")
		p.Out.URL.RawPath = ""
		p.Out.Host = target
		p.Out.Header.Del("Authorization")
		p.Out.Header.Del(shared.NativeAuthorizationHeader)
		p.Out.Header.Del("X-Workbench-Request")
		p.Out.Header.Del("Forwarded")
		p.Out.Header.Del("X-Forwarded-For")
		p.Out.Header.Del("X-Forwarded-Host")
		p.Out.Header.Del("X-Forwarded-Proto")
		p.Out.Header.Del("Cookie")
		for _, cookie := range p.In.Cookies() {
			if !strings.HasPrefix(cookie.Name, "awb_") {
				p.Out.AddCookie(cookie)
			}
		}
		if p.In.Header.Get("Origin") != "" {
			p.Out.Header.Set("Origin", "http://"+target)
		}
	}, ModifyResponse: func(res *http.Response) error {
		// Framework canonical redirects must stay on the isolated preview origin.
		if location, err := url.Parse(res.Header.Get("Location")); err == nil && location.Scheme == "http" && location.Port() == strconv.Itoa(app.Port) {
			host := location.Hostname()
			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				location.Scheme = ""
				location.Host = ""
				res.Header.Set("Location", location.String())
			}
		}
		cookies := res.Cookies()
		res.Header.Del("Set-Cookie")
		for _, cookie := range cookies {
			if strings.HasPrefix(cookie.Name, "awb_") {
				continue
			}
			cookie.Domain = ""
			res.Header.Add("Set-Cookie", cookie.String())
		}
		return nil
	}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) { shared.Error(w, 503, "APPLICATION_NOT_READY") }}
	proxy.ServeHTTP(w, r)
}
