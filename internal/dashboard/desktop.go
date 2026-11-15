package dashboard

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"agent-workbench/internal/shared"
)

func issueDesktopPair(w http.ResponseWriter, r *http.Request, client *http.Client, origin, token, id string) {
	req, err := http.NewRequestWithContext(r.Context(), "POST", origin+"/v1/workspaces/"+id+"/console/api/auth/pairing-token", strings.NewReader(`{"label":"Workbench desktop"}`))
	if err != nil {
		writeError(w, 503, "CONSOLE_UNAVAILABLE")
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		writeError(w, 503, "CONSOLE_UNAVAILABLE")
		return
	}
	defer res.Body.Close()
	var pairing struct {
		Credential string `json:"credential"`
		ExpiresAt  string `json:"expiresAt"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&pairing) != nil || pairing.Credential == "" {
		writeError(w, 503, "PAIRING_UNAVAILABLE")
		return
	}
	host := "desktop-" + strings.Replace(id, "ws_", "ws-", 1) + ".localhost"
	if _, port, err := net.SplitHostPort(r.Host); err == nil {
		host = net.JoinHostPort(host, port)
	}
	scheme := "http"
	if origin, ok := r.Context().Value(authenticatedOriginKey{}).(string); ok {
		if public, err := url.Parse(origin); err == nil && public.Scheme == "https" {
			host = "desktop-" + strings.Replace(id, "ws_", "ws-", 1) + "." + public.Host
			scheme = "https"
		}
	}
	u := url.URL{Scheme: scheme, Host: host, Path: "/pair", Fragment: url.Values{"token": {pairing.Credential}}.Encode()}
	shared.JSON(w, 200, map[string]string{"pairing_url": u.String(), "expires_at": pairing.ExpiresAt})
}
