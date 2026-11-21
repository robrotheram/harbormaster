package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
)

const previewAppCookiePrefix = "awbapp_"

func previewOrigin(r *http.Request) string {
	if workerSecure(r) {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}

func previewParents(r *http.Request) string {
	if label, ok := r.Context().Value(publicWorkerIngressKey{}).(string); ok {
		return "https://" + strings.TrimPrefix(r.Host, label+".")
	}
	host := r.Host
	_, port, _ := net.SplitHostPort(host)
	suffix := ""
	if port != "" {
		suffix = ":" + port
	}
	return "http://127.0.0.1" + suffix + " http://localhost" + suffix
}
func previewPolicy(r *http.Request) string {
	host := r.Host
	websocketScheme := "ws://"
	if workerSecure(r) {
		websocketScheme = "wss://"
	}
	return "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self' " + websocketScheme + host + "; font-src 'self' data:; media-src 'self' blob:; base-uri 'self'; form-action 'self'; frame-ancestors " + previewParents(r) + "; worker-src 'self' blob:"
}
func (p *previewIngress) live(w http.ResponseWriter, r *http.Request, binding previewBinding) {
	if binding.Embedded && r.Method == http.MethodGet && r.URL.Path == "/.workbench/preview-review.js" {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, frontendDir()+"/preview-review.js")
		return
	}
	// ReverseProxy adds response headers. Replace the connect-page policy, rather
	// than applying two CSPs and accidentally disabling the application's scripts.
	w.Header().Del("Content-Security-Policy")
	target, _ := url.Parse(p.origin)
	proxy := httputil.ReverseProxy{Transport: p.client.Transport, FlushInterval: -1, Rewrite: func(pr *httputil.ProxyRequest) {
		pr.Out.URL.Scheme = target.Scheme
		pr.Out.URL.Host = target.Host
		pr.Out.URL.Path = "/v1/workspaces/" + binding.Workspace + "/application-proxy/" + binding.Service + r.URL.Path
		pr.Out.URL.RawPath = ""
		pr.Out.Header.Set("Authorization", "Bearer "+humanToken(r, p.token))
		pr.Out.Header.Del("Accept-Encoding")
		pr.Out.Header.Del("Cookie")
		for _, cookie := range pr.In.Cookies() {
			name, ok := strings.CutPrefix(cookie.Name, previewAppCookiePrefix)
			if !ok || name == "" {
				continue
			}
			pr.Out.AddCookie(&http.Cookie{Name: name, Value: cookie.Value, Quoted: cookie.Quoted})
		}
	}, ModifyResponse: func(res *http.Response) error {
		res.Header.Set("Content-Security-Policy", previewPolicy(r))
		res.Header.Del("X-Frame-Options")
		res.Header.Set("Cache-Control", "no-store")
		res.Header.Set("Referrer-Policy", "no-referrer")
		// Inspection runs on the isolated preview origin, and only accepts activation
		// from the exact workbench parent window. Selected text becomes a draft.
		if binding.Embedded && strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") && res.Header.Get("Content-Encoding") == "" && res.StatusCode == 200 {
			data, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
			if err != nil {
				return err
			}
			if len(data) > 2<<20 {
				res.Body = struct {
					io.Reader
					io.Closer
				}{io.MultiReader(bytes.NewReader(data), res.Body), res.Body}
			} else {
				res.Body.Close()
				parent, _ := json.Marshal(binding.ParentOrigin)
				script := `<script>(()=>{const origin=` + string(parent) + `;let nonce='',selecting=false;window.addEventListener('message',e=>{if(e.source!==parent||e.origin!==origin||e.data?.type!=='workbench-select')return;nonce=String(e.data.nonce||'');selecting=!!e.data.enabled;document.documentElement.style.cursor=selecting?'crosshair':''});document.addEventListener('click',e=>{if(!selecting)return;e.preventDefault();e.stopImmediatePropagation();selecting=false;document.documentElement.style.cursor='';const el=e.target;let selector=el.id?'#'+CSS.escape(el.id):el.tagName.toLowerCase();parent.postMessage({type:'workbench-selection',nonce,selector,text:(el.innerText||el.getAttribute('aria-label')||'').slice(0,300),path:location.pathname},origin)},true)})()</script>`
				review := `<script defer src="/.workbench/preview-review.js" data-parent-origin="` + html.EscapeString(binding.ParentOrigin) + `"></script>`
				data = append(data, []byte(script+review)...)
				res.Body = io.NopCloser(bytes.NewReader(data))
				res.ContentLength = int64(len(data))
				res.Header.Set("Content-Length", strconv.Itoa(len(data)))
			}
		}
		cookies := res.Cookies()
		res.Header.Del("Set-Cookie")
		for _, cookie := range cookies {
			cookie.Name = previewAppCookiePrefix + cookie.Name
			cookie.Domain = ""
			if binding.Embedded {
				cookie.Secure = true
				cookie.SameSite = http.SameSiteNoneMode
				cookie.Partitioned = true
			}
			res.Header.Add("Set-Cookie", cookie.String())
		}
		return nil
	}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
		writeConnectionError(w, r, 503, "PREVIEW_UNAVAILABLE", "preview")
	}}
	// Expiry closes long-running upgraded connections as well as ordinary requests.
	ctx, cancel := context.WithDeadline(r.Context(), binding.Expiry)
	defer cancel()
	proxy.ServeHTTP(w, r.WithContext(ctx))
}
