package mcpserver

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Config struct {
	ControllerURL       string
	GatewayServiceToken string
	ControlplaneURL     string
	ControlToken        string
	MCPToken            string
	AllowedHosts        []string
}

// ConfigFromEnv only accepts file-provisioned credentials. It never falls back
// to inline environment tokens or caller-supplied control-plane identities.
func ConfigFromEnv() (Config, error) {
	c := Config{ControlplaneURL: os.Getenv("CONTROLPLANE_URL"), ControllerURL: os.Getenv("HERMES_CONTROLLER_URL")}
	if c.ControlplaneURL == "" {
		c.ControlplaneURL = "http://controlplane:8080"
	}
	read := func(name string) (string, error) {
		p := os.Getenv(name)
		if p == "" {
			return "", errors.New("missing credential file configuration")
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", errors.New("cannot read credential file")
		}
		s := strings.TrimSpace(string(b))
		if !validToken(s) {
			return "", errors.New("invalid credential file")
		}
		return s, nil
	}
	var err error
	if c.ControllerURL != "" {
		if c.GatewayServiceToken, err = read("GATEWAY_SERVICE_TOKEN_FILE"); err != nil {
			return c, err
		}
	}
	if c.ControlToken, err = read("CONTROL_TOKEN_FILE"); err != nil {
		return c, err
	}
	if c.MCPToken, err = read("MCP_TOKEN_FILE"); err != nil {
		return c, err
	}
	hosts := os.Getenv("MCP_ALLOWED_HOSTS")
	if hosts == "" {
		hosts = "localhost,127.0.0.1,mcp"
	}
	c.AllowedHosts = strings.Split(hosts, ",")
	return c, nil
}

func validToken(s string) bool {
	return len(s) > 0 && len(s) <= 8192 && !strings.ContainsAny(s, " \t\r\n")
}

func New(c Config) (http.Handler, error) {
	u, err := url.Parse(c.ControlplaneURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("invalid control-plane URL")
	}
	if !validToken(c.ControlToken) || !validToken(c.MCPToken) || c.ControlToken == c.MCPToken {
		return nil, errors.New("distinct valid control and MCP credentials required")
	}
	if len(c.AllowedHosts) == 0 {
		c.AllowedHosts = []string{"localhost", "127.0.0.1", "mcp"}
	}
	for i, h := range c.AllowedHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if !validAuthority(h) {
			return nil, errors.New("invalid allowed host")
		}
		c.AllowedHosts[i] = h
	}
	cp := &controlClient{base: strings.TrimRight(c.ControlplaneURL, "/"), token: c.ControlToken, client: &http.Client{Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	server := mcp.NewServer(&mcp.Implementation{Name: "agent-workbench", Version: "0.1.0"}, nil)
	registerTools(server, cp)
	projectServer := mcp.NewServer(&mcp.Implementation{Name: "workbench-project", Version: "1"}, nil)
	registerCoordinationTools(projectServer, cp)
	registerWorkTools(projectServer, cp)
	projectServer.RemoveTools("work_review")
	stream := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if r.URL.Path == "/project-mcp" {
			return projectServer
		}
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 512 << 10,
		PropagateRequestCancellation: true,
		// The outer middleware enforces the explicit Host allowlist on every
		// interface, including loopback and container-network requests.
		DisableLocalhostProtection: true,
	})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	authenticated := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !allowedAuthority(r.Host, c.AllowedHosts) || !allowedOrigin(r, c.AllowedHosts) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		a := r.Header.Values("Authorization")
		if len(a) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		credential := strings.TrimPrefix(a[0], "Bearer ")
		project, verifyErr := shared.VerifyProjectAccess(c.ControlToken, credential, time.Now())
		if subtle.ConstantTimeCompare([]byte(a[0]), []byte("Bearer "+c.MCPToken)) == 1 {
			project = "default"
			credential, verifyErr = shared.IssueProjectAccess(c.ControlToken, project, time.Now().Add(time.Hour))
		}
		if verifyErr != nil || project == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), accessContextKey{}, projectAccess{token: credential, project: project}))
		r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
		stream.ServeHTTP(w, r)
	})
	mux.Handle("/mcp", authenticated)
	mux.Handle("/project-mcp", authenticated)
	if c.ControllerURL != "" {
		h, err := generationMCP(c, authenticated)
		if err != nil {
			return nil, err
		}
		mux.Handle("/hermes-mcp", h)
	}
	return mux, nil
}

func validAuthority(s string) bool {
	if s == "" || strings.ContainsAny(s, "*/@?#\\ \t\r\n") {
		return false
	}
	u, e := url.Parse("http://" + s)
	if e != nil || u.Host != s || u.Hostname() == "" {
		return false
	}
	return true
}
func allowedAuthority(s string, hosts []string) bool {
	s = strings.ToLower(s)
	if !validAuthority(s) {
		return false
	}
	for _, h := range hosts {
		if s == h {
			return true
		}
		// A hostname-only entry permits any explicit local listening port; an
		// entry with a port is exact and never widens to other ports.
		if !strings.Contains(h, ":") {
			host, _, e := net.SplitHostPort(s)
			if e == nil && host == h {
				return true
			}
		}
	}
	return false
}
func allowedOrigin(r *http.Request, hosts []string) bool {
	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return true
	}
	if len(values) != 1 {
		return false
	}
	u, e := url.Parse(values[0])
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && allowedAuthority(u.Host, hosts)
}

func boundedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 40*time.Second)
}
