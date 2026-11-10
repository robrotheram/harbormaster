package dashboard

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"agent-workbench/internal/shared"
)

// previewIngress has no target supplied by the browser. Its only byte source is
// the authenticated bounded service fetch API, which checks revocation on every request.
type previewBinding struct {
	ServiceExpiry                     time.Time
	Workspace, Service, Host, Project string
	Expiry                            time.Time
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
	req.Header.Set("Authorization", "Bearer "+p.token)
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
		State  string `json:"state"`
		Expiry int64  `json:"expires_at"`
	}
	if status != 200 || json.Unmarshal(b, &state) != nil || state.State != "active" || state.Expiry <= time.Now().Unix() {
		writeError(w, 410, "PREVIEW_UNAVAILABLE")
		return
	}
	_, port, _ := net.SplitHostPort(r.Host)
	host := previewHost(workspace, service, port)
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.tickets {
		if time.Now().After(v.Expiry) {
			delete(p.tickets, k)
		}
	}
	for k, v := range p.sessions {
		if time.Now().After(v.Expiry) {
			delete(p.sessions, k)
		}
	}
	if len(p.tickets)+len(p.sessions) >= 512 {
		writeError(w, 429, "TOO_MANY_CONNECTIONS")
		return
	}
	key := shared.NewID("ticket_")
	p.tickets[key] = previewBinding{Workspace: workspace, Service: service, Host: host, Project: owner.Project, Expiry: time.Now().Add(time.Minute), ServiceExpiry: time.Unix(state.Expiry, 0)}
	shared.JSON(w, 200, map[string]string{"preview_url": "http://" + host + "/connect#ticket=" + url.QueryEscape(key)})
}
func (p *previewIngress) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; font-src 'self'; media-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; worker-src 'none'")
	if r.Header.Get("Upgrade") != "" {
		writeError(w, 400, "INVALID_REQUEST")
		return
	}
	if r.URL.Path == "/connect" && r.Method == "GET" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>Open preview</title><script src="/connect.js" defer></script><p id="status">Opening retained preview…</p>`)
		return
	}
	if r.URL.Path == "/connect.js" && r.Method == "GET" {
		w.Header().Set("Content-Type", "text/javascript")
		io.WriteString(w, `(async()=>{const ticket=new URLSearchParams(location.hash.slice(1)).get('ticket');history.replaceState(null,'','/connect');const r=await fetch('/connect',{method:'POST',headers:{'Content-Type':'application/json','X-Workbench-Request':'1'},body:JSON.stringify({ticket})});if(r.ok)location.replace('/');else document.getElementById('status').textContent='Preview link expired. Open it again from the workbench.'})()`)
		return
	}
	if r.URL.Path == "/connect" && r.Method == "POST" {
		if r.Header.Get("Origin") != "http://"+r.Host || r.Header.Get("X-Workbench-Request") != "1" {
			writeError(w, 403, "ORIGIN_REJECTED")
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
			writeError(w, 403, "CONNECTION_EXPIRED")
			return
		}
		binding.Expiry = time.Now().Add(time.Hour)
		if binding.ServiceExpiry.Before(binding.Expiry) {
			binding.Expiry = binding.ServiceExpiry
		}
		key := shared.NewID("session_")
		p.sessions[key] = binding
		p.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "awb_preview", Value: key, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: binding.Expiry})
		shared.JSON(w, 200, map[string]bool{"connected": true})
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		writeError(w, 403, "ORIGIN_REJECTED")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		writeError(w, 405, "METHOD_NOT_ALLOWED")
		return
	}
	cookie, err := r.Cookie("awb_preview")
	if err != nil {
		writeError(w, 401, "OPEN_FROM_WORKBENCH")
		return
	}
	p.mu.Lock()
	binding, ok := p.sessions[cookie.Value]
	p.mu.Unlock()
	if !ok || binding.Host != r.Host || time.Now().After(binding.Expiry) {
		writeError(w, 401, "SESSION_EXPIRED")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" || strings.HasSuffix(name, "/") {
		name += "index.html"
	}
	if !shared.ValidProjectFile(name) || path.Clean(name) != name {
		writeError(w, 400, "INVALID_PATH")
		return
	}
	payload, _ := json.Marshal(map[string]string{"path": name})
	status, b := p.call(r, "POST", "/workspaces/"+binding.Workspace+"/services/"+binding.Service+"/fetch", payload)
	if status != 200 {
		writeError(w, status, "PREVIEW_UNAVAILABLE")
		return
	}
	var out struct {
		Content []byte `json:"content"`
		Type    string `json:"content_type"`
	}
	if json.Unmarshal(b, &out) != nil || len(out.Content) > 2<<20 {
		writeError(w, 503, "INVALID_PREVIEW_RESPONSE")
		return
	}
	kind := strings.Split(out.Type, ";")[0]
	switch kind {
	case "text/html", "text/css", "text/javascript", "application/javascript", "application/json", "text/plain", "image/png", "image/jpeg", "image/gif", "image/webp", "image/svg+xml", "image/x-icon", "font/woff", "font/woff2":
	default:
		writeError(w, 415, "UNSUPPORTED_PREVIEW_CONTENT")
		return
	}
	w.Header().Set("Content-Type", out.Type)
	if r.Method == "GET" {
		w.Write(out.Content)
	}
}
