package shared

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

var proxyTransport = &http.Transport{Proxy: nil, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 30 * time.Second}

// Set by the loopback desktop ingress from native client Authorization only.
// Management hops use their own credentials; the workspace restores this one.
const NativeAuthorizationHeader = "X-Workbench-Native-Authorization"

// Proxy targets are selected by trusted service configuration or a verified
// managed-container address. Browsers never choose a target address or token.
func Proxy(w http.ResponseWriter, r *http.Request, origin, path, token string) {
	proxyAuthorization(w, r, origin, path, "Bearer "+token)
}

func ProxyNative(w http.ResponseWriter, r *http.Request, origin, path, authorization string) {
	proxyAuthorization(w, r, origin, path, authorization)
}

func proxyAuthorization(w http.ResponseWriter, r *http.Request, origin, path, authorization string) {
	target, err := url.Parse(origin)
	if err != nil || target.Host == "" {
		Error(w, 503, "CONSOLE_UNAVAILABLE")
		return
	}
	proxy := &httputil.ReverseProxy{FlushInterval: -1, Transport: proxyTransport, Rewrite: func(p *httputil.ProxyRequest) {
		p.Out.URL.Scheme = target.Scheme
		p.Out.URL.Host = target.Host
		p.Out.URL.Path = path
		p.Out.URL.RawPath = ""
		p.Out.Host = p.In.Host
		p.Out.Header.Del("Forwarded")
		p.Out.Header.Del("X-Forwarded-For")
		p.Out.Header.Del("X-Forwarded-Host")
		p.Out.Header.Del("X-Forwarded-Proto")
		p.Out.Header.Set("X-Forwarded-Host", p.In.Host)
		p.Out.Header.Set("X-Forwarded-Proto", "http")
		p.Out.Header.Del("Authorization")
		if authorization != "" {
			p.Out.Header.Set("Authorization", authorization)
		}
	}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) { Error(w, 503, "CONSOLE_UNAVAILABLE") }}
	proxy.ServeHTTP(w, r)
}
