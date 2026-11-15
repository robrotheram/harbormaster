package dashboard

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"agent-workbench/internal/shared"
)

// OAuthProxyConfig integrates an operator-owned OAuth2 Proxy. The dashboard
// checks the session itself; client-supplied identity headers grant no access.
// This gates the shared operator console, not per-project authorization.
type OAuthProxyConfig struct{ Origin, AuthURL, PublicURL string }

func NewOAuthProxyHandler(next http.Handler, cfg OAuthProxyConfig) (http.Handler, error) {
	origin, err := url.Parse(cfg.Origin)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.ForceQuery {
		return nil, errors.New("dashboard public origin must be an HTTPS origin")
	}
	public, err := url.Parse(cfg.PublicURL)
	if err != nil || public.Scheme != "https" || public.Host == "" || public.User != nil || public.Path != "" || public.RawQuery != "" || public.Fragment != "" || public.ForceQuery {
		return nil, errors.New("OAuth2 Proxy public URL must be an HTTPS origin")
	}
	auth, err := url.Parse(cfg.AuthURL)
	if err != nil || (auth.Scheme != "https" && auth.Scheme != "http") || auth.Host == "" || auth.User != nil || auth.Path != "/oauth2/auth" || auth.RawQuery != "" || auth.Fragment != "" || auth.ForceQuery {
		return nil, errors.New("OAuth2 Proxy auth URL must end in /oauth2/auth")
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil, MaxResponseHeaderBytes: 32 << 10}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	login := cfg.PublicURL + "/oauth2/start?" + url.Values{"rd": {cfg.Origin + "/app/"}}.Encode()
	logout := cfg.PublicURL + "/oauth2/sign_out?" + url.Values{"rd": {cfg.Origin + "/app/"}}.Encode()
	var logoutMu sync.Mutex
	logoutTickets := map[string]time.Time{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
			shared.JSON(w, 200, map[string]string{"status": "ok"})
			return
		}
		if !strings.EqualFold(r.Host, origin.Host) {
			writeError(w, 403, "HOST_REJECTED")
			return
		}
		if incoming := r.Header.Get("Origin"); incoming != "" && incoming != cfg.Origin {
			writeError(w, 403, "ORIGIN_REJECTED")
			return
		}
		// Keep browser navigation on the frontend's same-origin allowlist. Only
		// a successful CSRF-protected logout POST can issue this one-use redirect.
		if r.URL.Path == "/auth/proxy-signout" {
			key := r.URL.Query().Get("ticket")
			logoutMu.Lock()
			expires, ok := logoutTickets[key]
			delete(logoutTickets, key)
			logoutMu.Unlock()
			if r.Method != http.MethodGet || !ok || !time.Now().Before(expires) {
				writeError(w, 403, "INVALID_LOGOUT_TICKET")
				return
			}
			http.Redirect(w, r, logout, http.StatusSeeOther)
			return
		}
		if r.URL.Path == "/auth/login" {
			if r.Method != http.MethodGet {
				writeError(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
			http.Redirect(w, r, login, http.StatusSeeOther)
			return
		}
		if r.URL.Path == "/auth/session" && r.Method != http.MethodGet {
			writeError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if r.URL.Path == "/auth/logout" && (r.Method != http.MethodPost || r.Header.Get("Origin") != cfg.Origin || r.Header.Get("X-Workbench-Request") != "1") {
			writeError(w, 403, "ORIGIN_REJECTED")
			return
		}
		check, err := http.NewRequestWithContext(r.Context(), http.MethodGet, cfg.AuthURL, nil)
		if err != nil {
			writeError(w, 503, "AUTH_UNAVAILABLE")
			return
		}
		check.Header.Set("Cookie", r.Header.Get("Cookie"))
		check.Header.Set("X-Forwarded-Host", origin.Host)
		check.Header.Set("X-Forwarded-Proto", "https")
		check.Header.Set("X-Forwarded-Uri", r.URL.RequestURI())
		check.Header.Set("X-Forwarded-Method", r.Method)
		response, err := client.Do(check)
		if err != nil {
			writeError(w, 503, "AUTH_UNAVAILABLE")
			return
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode == 401 || response.StatusCode == 403 {
			if r.URL.Path == "/auth/session" {
				shared.JSON(w, 200, map[string]string{"status": "anonymous"})
			} else {
				writeError(w, 401, "AUTH_REQUIRED")
			}
			return
		}
		if response.StatusCode != 200 && response.StatusCode != 202 {
			writeError(w, 503, "AUTH_UNAVAILABLE")
			return
		}
		// A successful response without an identity is a proxy configuration error.
		id := response.Header.Get("X-Auth-Request-User")
		email := response.Header.Get("X-Auth-Request-Email")
		if id == "" {
			id = email
		}
		if id == "" {
			writeError(w, 503, "AUTH_IDENTITY_UNAVAILABLE")
			return
		}
		for _, cookie := range response.Header.Values("Set-Cookie") {
			w.Header().Add("Set-Cookie", cookie)
		}
		if r.URL.Path == "/auth/session" {
			name := response.Header.Get("X-Auth-Request-Preferred-Username")
			if name == "" {
				name = email
			}
			if name == "" {
				name = id
			}
			shared.JSON(w, 200, map[string]any{"status": "authenticated", "user": map[string]string{"id": id, "name": name, "email": email}})
			return
		}
		if r.URL.Path == "/auth/logout" {
			key := shared.NewID("logout_")
			logoutMu.Lock()
			for ticket, expires := range logoutTickets {
				if !time.Now().Before(expires) {
					delete(logoutTickets, ticket)
				}
			}
			if len(logoutTickets) >= 256 {
				logoutMu.Unlock()
				writeError(w, 429, "TOO_MANY_LOGOUT_REQUESTS")
				return
			}
			logoutTickets[key] = time.Now().Add(time.Minute)
			logoutMu.Unlock()
			shared.JSON(w, 200, map[string]string{"url": "/auth/proxy-signout?ticket=" + key})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authenticatedOriginKey{}, cfg.Origin)))
	}), nil
}
