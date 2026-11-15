package dashboard

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-workbench/internal/shared"
)

// The local single-operator launcher is a separate surface from the read-only
// operational snapshot. It has a control credential, but no runtime/Vault access.
func NewInteractiveHandler(readURL, controlURL, token string) (http.Handler, error) {
	read, err := NewHandler(readURL)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(controlURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" || u.RawQuery != "" || token == "" {
		return nil, errors.New("invalid launcher upstream")
	}
	type ticket struct {
		id      string
		expires time.Time
	}
	var ticketsMu sync.Mutex
	tickets := map[string]ticket{}
	client := &http.Client{Timeout: 35 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	previews := newPreviewIngress(client, controlURL, token)
	sign := func(id, expires string) string {
		mac := hmac.New(sha256.New, []byte(token))
		mac.Write([]byte("console:" + id + ":" + expires))
		return expires + "." + hex.EncodeToString(mac.Sum(nil))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(r.Host)
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if label, ok := r.Context().Value(publicWorkerIngressKey{}).(string); ok {
			host = label + ".localhost"
		}
		desktop := strings.HasPrefix(host, "desktop-ws-") && strings.HasSuffix(host, ".localhost")
		if origin := r.Header.Get("Origin"); origin != "" {
			parsed, e := url.Parse(origin)
			nativeOrigin := desktop && (origin == "t3code://app" || origin == "t3code-dev://app")
			if !nativeOrigin && (e != nil || (parsed.Scheme != "http" && r.Context().Value(authenticatedOriginKey{}) != origin) || !strings.EqualFold(parsed.Host, r.Host)) {
				writeError(w, 403, "ORIGIN_REJECTED")
				return
			}
		}
		if desktop {
			id := strings.Replace(strings.TrimSuffix(strings.TrimPrefix(host, "desktop-"), ".localhost"), "ws-", "ws_", 1)
			if !shared.ValidID(id) || !strings.HasPrefix(id, "ws_") {
				writeError(w, 404, "NOT_FOUND")
				return
			}
			r.Header.Set(shared.NativeAuthorizationHeader, r.Header.Get("Authorization"))
			r.Header.Del("Cookie")
			w.Header().Set("Cache-Control", "no-store")
			shared.Proxy(w, r, controlURL, "/v1/workspaces/"+id+"/desktop"+r.URL.Path, token)
			return
		}
		if strings.HasPrefix(host, "preview-") && strings.HasSuffix(host, ".localhost") {
			previews.serve(w, r)
			return
		}
		if strings.HasSuffix(host, ".localhost") {
			label := strings.TrimSuffix(host, ".localhost")
			id := strings.Replace(label, "ws-", "ws_", 1)
			if !strings.HasPrefix(label, "ws-") || !shared.ValidID(id) || !strings.HasPrefix(id, "ws_") {
				writeError(w, 404, "NOT_FOUND")
				return
			}
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Cache-Control", "no-store")
			if serveConnectionAsset(w, r) {
				return
			}
			if r.URL.Path == "/connect" && r.Method == "GET" {
				serveConnectionPage(w, r, "worker", "", http.StatusOK)
				return
			}
			if r.URL.Path == "/connect" && r.Method == "POST" {
				if r.Header.Get("X-Workbench-Request") != "1" {
					writeError(w, 403, "ORIGIN_REJECTED")
					return
				}
				var input struct {
					Ticket string `json:"ticket"`
				}
				if !shared.Decode(w, r, &input, 1024) {
					return
				}
				ticketsMu.Lock()
				issued, ok := tickets[input.Ticket]
				delete(tickets, input.Ticket)
				ticketsMu.Unlock()
				if !ok || issued.id != id || time.Now().After(issued.expires) {
					writeError(w, 403, "CONNECTION_EXPIRED")
					return
				}
				expiry := time.Now().Add(12 * time.Hour)
				http.SetCookie(w, &http.Cookie{Name: "awb_console", Value: sign(id, strconv.FormatInt(expiry.Unix(), 10)), Path: "/", HttpOnly: true, Secure: workerSecure(r), SameSite: http.SameSiteStrictMode, Expires: expiry})
				shared.JSON(w, 200, map[string]bool{"connected": true})
				return
			}
			cookie, e := r.Cookie("awb_console")
			valid := false
			if e == nil {
				parts := strings.Split(cookie.Value, ".")
				if len(parts) == 2 {
					expires, e := strconv.ParseInt(parts[0], 10, 64)
					valid = e == nil && expires > time.Now().Unix() && hmac.Equal([]byte(cookie.Value), []byte(sign(id, parts[0])))
				}
			}
			if !valid {
				writeConnectionError(w, r, 401, "OPEN_FROM_WORKSPACE_LAUNCHER", "worker")
				return
			}
			shared.Proxy(w, r, controlURL, "/v1/workspaces/"+id+"/console"+r.URL.Path, token)
			return
		}
		if host != "127.0.0.1" && host != "localhost" && r.Context().Value(authenticatedOriginKey{}) == nil {
			writeError(w, 403, "HOST_REJECTED")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if serveFrontend(w, r) {
			return
		}
		if strings.HasPrefix(r.URL.Path, "/ui-api/") {
			r.Header.Del("X-Workbench-Subscription-User")
			if identity, ok := r.Context().Value(subscriptionIdentityKey{}).(string); ok {
				r.Header.Set("X-Workbench-Subscription-User", identity)
			}
			path := strings.TrimPrefix(r.URL.Path, "/ui-api")
			parts := strings.Split(strings.Trim(path, "/"), "/")
			query := r.URL.Query()
			validQuery := true
			for key, values := range query {
				if key == "page" && len(values) == 1 && r.Method == "GET" && len(parts) == 3 && parts[0] == "forge-connections" && parts[2] == "issues" {
					continue
				}
				if key == "episode" && len(values) == 1 && shared.ValidID(values[0]) && r.Method == "GET" && len(parts) == 3 && parts[0] == "workspaces" && parts[2] == "history" {
					continue
				}
				if key != "project_id" && key != "offset" && key != "limit" && key != "after" && key != "before" && key != "until" && key != "order" || len(values) != 1 {
					validQuery = false
				}
			}
			if !validQuery {
				writeError(w, 400, "INVALID_REQUEST")
				return
			}
			if len(parts) == 5 && parts[0] == "workspaces" && parts[2] == "services" && parts[4] == "connect" && r.Method == "POST" {
				if r.Header.Get("X-Workbench-Request") != "1" {
					writeError(w, 403, "ORIGIN_REJECTED")
					return
				}
				previews.issue(w, r, parts[1], parts[3])
				return
			}
			if !allowedUIRoute(r.Method, path) {
				writeError(w, 404, "NOT_FOUND")
				return
			}
			if r.Method == "POST" && (r.Header.Get("X-Workbench-Request") != "1" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")) {
				writeError(w, 403, "ORIGIN_REJECTED")
				return
			}
			limit := int64(4096)
			if path == "/subscription" {
				limit = 70 << 10
			}
			if parts[0] == "work-items" && len(parts) == 1 {
				limit = 400000
			}
			if parts[0] == "note-publications" || (parts[0] == "project-exchanges" && len(parts) == 3 && parts[2] == "exports") {
				limit = 100000
			}
			if parts[0] == "connection-profiles" || len(parts) >= 3 && (parts[2] == "settings" || parts[2] == "hermes") {
				limit = 32768
			}
			if len(parts) >= 3 && (parts[2] == "tasks" || parts[2] == "terminals") {
				limit = 70 << 10
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			if len(parts) == 3 && (parts[2] == "start" || parts[2] == "desktop-pair") {
				payload, e := io.ReadAll(r.Body)
				if e != nil {
					writeError(w, 400, "INVALID_REQUEST")
					return
				}
				request, e := http.NewRequestWithContext(r.Context(), "POST", controlURL+"/v1/workspaces/"+parts[1]+"/start", bytes.NewReader(payload))
				if e != nil {
					writeError(w, 503, "CONSOLE_UNAVAILABLE")
					return
				}
				request.Header.Set("Authorization", "Bearer "+token)
				request.Header.Set("Content-Type", "application/json")
				response, e := client.Do(request)
				if e != nil {
					writeError(w, 503, "CONSOLE_UNAVAILABLE")
					return
				}
				defer response.Body.Close()
				data, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
				if e != nil || len(data) > 1<<20 {
					writeError(w, 503, "CONSOLE_UNAVAILABLE")
					return
				}
				var result map[string]any
				if json.Unmarshal(data, &result) != nil {
					writeError(w, 503, "CONSOLE_UNAVAILABLE")
					return
				}
				if response.StatusCode == 200 {
					if parts[2] == "desktop-pair" {
						issueDesktopPair(w, r, client, controlURL, token, parts[1])
						return
					}
					key := shared.NewID("connect_")
					ticketsMu.Lock()
					for k, t := range tickets {
						if time.Now().After(t.expires) {
							delete(tickets, k)
						}
					}
					if len(tickets) >= 256 {
						ticketsMu.Unlock()
						writeError(w, 429, "TOO_MANY_CONNECTIONS")
						return
					}
					tickets[key] = ticket{parts[1], time.Now().Add(time.Minute)}
					ticketsMu.Unlock()
					result["console_ticket"] = key
				}
				shared.JSON(w, response.StatusCode, result)
				return
			}
			shared.Proxy(w, r, controlURL, "/v1"+path, token)
			return
		}

		read.ServeHTTP(w, r)
	}), nil
}
