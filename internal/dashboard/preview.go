package dashboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"agent-workbench/internal/shared"
)

// previewIngress has no target supplied by the browser. Its only byte source is
// authenticated workspace services, which check revocation on every request.
type previewBinding struct {
	Human                             *shared.HumanIdentity
	session                           context.Context
	cancel                            context.CancelFunc
	Embedded                          bool
	ParentOrigin                      string
	Application                       bool
	Desktop                           bool
	ServiceExpiry                     time.Time
	Workspace, Service, Host, Project string
	Expiry                            time.Time
}

func (b previewBinding) close() {
	if b.cancel != nil {
		b.cancel()
	}
}

func (p *previewIngress) revoke(user string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, entries := range []map[string]previewBinding{p.tickets, p.sessions} {
		for key, binding := range entries {
			if binding.Human != nil && binding.Human.ID == user {
				binding.close()
				delete(entries, key)
			}
		}
	}
}

func previewConnectionBinding(p previewBinding) (*shared.HumanIdentity, time.Time) {
	return p.Human, p.Expiry
}

type previewIngress struct {
	mu                sync.Mutex
	tickets, sessions map[string]previewBinding
	client            *http.Client
	origin, token     string
}

func newPreviewIngress(client *http.Client, origin, token string) *previewIngress {
	return &previewIngress{tickets: map[string]previewBinding{}, sessions: map[string]previewBinding{}, client: client, origin: origin, token: token}
}
func (p *previewIngress) call(r *http.Request, method, endpoint string, body []byte) (int, []byte) {
	req, err := http.NewRequestWithContext(r.Context(), method, p.origin+"/v1"+endpoint, bytes.NewReader(body))
	if err != nil {
		return 503, nil
	}
	req.Header.Set("Authorization", "Bearer "+humanToken(r, p.token))
	req.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return 503, nil
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, (3<<20)+1))
	if err != nil || len(b) > 3<<20 {
		return 503, nil
	}
	return res.StatusCode, b
}
func previewHost(workspace, service, port string) string {
	digest := sha256.Sum256([]byte(workspace + ":" + service))
	host := fmt.Sprintf("preview-%x.localhost", digest[:16])
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	return host
}
func (p *previewIngress) issue(w http.ResponseWriter, r *http.Request, workspace, service string) {
	var input struct {
		Embedded bool `json:"embedded"`
	}
	if !shared.Decode(w, r, &input, 1024) {
		return
	}
	if !shared.ValidID(workspace) || !shared.ValidID(service) {
		writeError(w, 400, "INVALID_REQUEST")
		return
	}
	status, metadata := p.call(r, "GET", "/workspaces/"+workspace, nil)
	var owner struct {
		Project string `json:"project_id"`
	}
	if status != 200 || json.Unmarshal(metadata, &owner) != nil || !shared.ValidID(owner.Project) {
		writeError(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return
	}
	status, b := p.call(r, "GET", "/workspaces/"+workspace+"/services/"+service, nil)
	var state struct {
		Kind            string `json:"kind"`
		ApplicationKind string `json:"application_kind"`
		State           string `json:"state"`
		Expiry          int64  `json:"expires_at"`
	}
	if status != 200 || json.Unmarshal(b, &state) != nil || state.State != "active" || state.Expiry <= time.Now().Unix() {
		writeError(w, 410, "PREVIEW_UNAVAILABLE")
		return
	}
	_, port, _ := net.SplitHostPort(r.Host)
	host := previewHost(workspace, service, port)
	parentOrigin := "http://" + r.Host
	scheme := "http"
	// Only the authenticated dashboard origin determines public preview URLs.
	// Forwarded headers and browser-supplied targets are not trusted.
	if origin, ok := r.Context().Value(authenticatedOriginKey{}).(string); ok && strings.HasPrefix(origin, "https://") {
		parentOrigin = origin
		scheme = "https"
		host = strings.TrimSuffix(previewHost(workspace, service, ""), ".localhost") + "." + strings.TrimPrefix(origin, "https://")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var human *shared.HumanIdentity
	if h, ok := r.Context().Value(humanIdentityKey{}).(shared.HumanIdentity); ok {
		human = &h
	}
	limitConnections(p.tickets, human, 8, previewConnectionBinding, nil)
	limitConnections(p.sessions, nil, 0, previewConnectionBinding, previewBinding.close)
	if len(p.tickets)+len(p.sessions) >= 512 {
		writeError(w, 429, "TOO_MANY_CONNECTIONS")
		return
	}
	key := shared.NewID("ticket_")
	p.tickets[key] = previewBinding{Human: human, Embedded: input.Embedded, ParentOrigin: parentOrigin, Application: state.Kind == "application", Desktop: state.ApplicationKind == "desktop", Workspace: workspace, Service: service, Host: host, Project: owner.Project, Expiry: time.Now().Add(time.Minute), ServiceExpiry: time.Unix(state.Expiry, 0)}
	shared.JSON(w, 200, map[string]string{"preview_url": scheme + "://" + host + "/connect#ticket=" + url.QueryEscape(key)})
}
func (p *previewIngress) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; font-src 'self'; media-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors "+previewParents(r)+"; worker-src 'none'")
	if serveConnectionAsset(w, r) {
		return
	}
	if r.URL.Path == "/connect" && r.Method == "GET" {
		serveConnectionPage(w, r, "preview", "", http.StatusOK)
		return
	}
	if r.URL.Path == "/connect" && r.Method == "POST" {
		if r.Header.Get("Origin") != previewOrigin(r) || r.Header.Get("X-Workbench-Request") != "1" {
			writeConnectionError(w, r, 403, "ORIGIN_REJECTED", "preview")
			return
		}
		var in struct {
			Ticket string `json:"ticket"`
		}
		if !shared.Decode(w, r, &in, 1024) {
			return
		}
		p.mu.Lock()
		binding, ok := p.tickets[in.Ticket]
		delete(p.tickets, in.Ticket)
		if !ok || binding.Host != r.Host || time.Now().After(binding.Expiry) {
			p.mu.Unlock()
			writeConnectionError(w, r, 403, "CONNECTION_EXPIRED", "preview")
			return
		}
		binding.Expiry = time.Now().Add(time.Hour)
		if binding.ServiceExpiry.Before(binding.Expiry) {
			binding.Expiry = binding.ServiceExpiry
		}
		key := shared.NewID("session_")
		limitConnections(p.sessions, binding.Human, 16, previewConnectionBinding, previewBinding.close)
		binding.session, binding.cancel = context.WithDeadline(context.Background(), binding.Expiry)
		p.sessions[key] = binding
		p.mu.Unlock()
		cookie := &http.Cookie{Name: hostCookieName("awb_preview", workerSecure(r)), Value: key, Path: "/", HttpOnly: true, Secure: workerSecure(r), SameSite: http.SameSiteStrictMode, Expires: binding.Expiry}
		if binding.Embedded {
			cookie.Secure = true
			cookie.SameSite = http.SameSiteNoneMode
			cookie.Partitioned = true
		}
		http.SetCookie(w, cookie)
		shared.JSON(w, 200, map[string]bool{"connected": true})
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		writeConnectionError(w, r, 403, "ORIGIN_REJECTED", "preview")
		return
	}
	cookie, err := uniqueCookie(r, hostCookieName("awb_preview", workerSecure(r)))
	if err != nil {
		writeConnectionError(w, r, 401, "OPEN_FROM_WORKBENCH", "preview")
		return
	}
	p.mu.Lock()
	binding, ok := p.sessions[cookie.Value]
	p.mu.Unlock()
	if !ok || binding.Host != r.Host || time.Now().After(binding.Expiry) {
		writeConnectionError(w, r, 401, "SESSION_EXPIRED", "preview")
		return
	}
	if binding.Human != nil {
		r = r.WithContext(context.WithValue(r.Context(), humanIdentityKey{}, *binding.Human))
	}
	if binding.session != nil {
		var done func()
		r, done = withWorkerSession(r, binding.session)
		defer done()
	}
	if binding.Application {
		if (r.Header.Get("Upgrade") != "" || (r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS")) && r.Header.Get("Origin") != previewOrigin(r) {
			writeConnectionError(w, r, 403, "ORIGIN_REJECTED", "preview")
			return
		}
		p.live(w, r, binding)
		return
	}
	if r.Header.Get("Upgrade") != "" {
		writeConnectionError(w, r, 400, "INVALID_REQUEST", "preview")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		writeConnectionError(w, r, 405, "METHOD_NOT_ALLOWED", "preview")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" || strings.HasSuffix(name, "/") {
		name += "index.html"
	}
	if !shared.ValidProjectFile(name) {
		writeConnectionError(w, r, 400, "INVALID_PATH", "preview")
		return
	}
	payload, _ := json.Marshal(map[string]string{"path": name})
	status, b := p.call(r, "POST", "/workspaces/"+binding.Workspace+"/services/"+binding.Service+"/fetch", payload)
	if status != 200 {
		writeConnectionError(w, r, status, "PREVIEW_UNAVAILABLE", "preview")
		return
	}
	var out struct {
		Content []byte `json:"content"`
		Type    string `json:"content_type"`
	}
	if json.Unmarshal(b, &out) != nil || len(out.Content) > 2<<20 {
		writeConnectionError(w, r, 503, "INVALID_PREVIEW_RESPONSE", "preview")
		return
	}
	kind := strings.Split(out.Type, ";")[0]
	switch kind {
	case "application/wasm", "application/octet-stream", "text/html", "text/css", "text/javascript", "application/javascript", "application/json", "text/plain", "image/png", "image/jpeg", "image/gif", "image/webp", "image/svg+xml", "image/x-icon", "font/woff", "font/woff2":
	default:
		writeConnectionError(w, r, 415, "UNSUPPORTED_PREVIEW_CONTENT", "preview")
		return
	}
	w.Header().Set("Content-Type", out.Type)
	if r.Method != "HEAD" {
		w.Write(out.Content)
	}
}
