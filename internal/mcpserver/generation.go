package mcpserver

import (
	"agent-workbench/internal/shared"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// generationMCP trusts only the operator-configured controller. Guest headers
// never choose a controller, project, upstream, or signing authority.
func generationMCP(c Config, next http.Handler) (http.Handler, error) {
	u, err := url.Parse(c.ControllerURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || !validToken(c.GatewayServiceToken) {
		return nil, errors.New("invalid Hermes controller configuration")
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "POST" || r.URL.RawQuery != "" || !allowedAuthority(r.Host, c.AllowedHosts) || !allowedOrigin(r, c.AllowedHosts) {
			http.Error(w, "forbidden", 403)
			return
		}
		auth := r.Header.Values("Authorization")
		if len(auth) != 1 || !strings.HasPrefix(auth[0], "Bearer ") || len(strings.TrimPrefix(auth[0], "Bearer ")) != 64 {
			http.Error(w, "unauthorized", 401)
			return
		}
		payload, _ := json.Marshal(map[string]string{"token": strings.TrimPrefix(auth[0], "Bearer "), "audience": "hermes-mcp"})
		req, _ := http.NewRequestWithContext(r.Context(), "POST", c.ControllerURL+"/v1/grants/verify", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+c.GatewayServiceToken)
		req.Header.Set("Content-Type", "application/json")
		res, e := client.Do(req)
		if e != nil {
			http.Error(w, "verifier unavailable", 503)
			return
		}
		defer res.Body.Close()
		var identity struct {
			Project   string `json:"project_id"`
			Workspace string `json:"workspace_id"`
		}
		if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&identity) != nil || !shared.ValidID(identity.Project) || !shared.ValidID(identity.Workspace) {
			http.Error(w, "unauthorized", 401)
			return
		}
		body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
		if e != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		var rpc struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if json.Unmarshal(body, &rpc) != nil {
			http.Error(w, "invalid RPC", 400)
			return
		}
		switch rpc.Method {
		case "initialize", "notifications/initialized", "ping", "tools/list":
		case "tools/call":
			if !shared.ProjectToolAllowed(rpc.Params.Name) {
				http.Error(w, "forbidden tool", 403)
				return
			}
		default:
			http.Error(w, "forbidden method", 403)
			return
		}
		token, e := shared.IssueProjectAccess(c.ControlToken, identity.Project, time.Now().Add(time.Minute))
		if e != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		forwarded := r.Clone(r.Context())
		forwarded.Header = r.Header.Clone()
		forwarded.Header.Set("Authorization", "Bearer "+token)
		forwarded.URL.Path = "/project-mcp"
		forwarded.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, forwarded)
	}), nil
}
