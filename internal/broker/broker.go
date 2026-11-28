// Package broker implements narrowly scoped, principal-bound mTLS operations.
package broker

import (
	"agent-workbench/internal/gateway"
	"agent-workbench/internal/shared"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type Parameter struct {
	Type      string   `json:"type"` // string, number, boolean
	Required  bool     `json:"required,omitempty"`
	MaxLength int      `json:"max_length,omitempty"`
	Enum      []string `json:"enum,omitempty"`
}
type Operation struct {
	Origin     string                `json:"origin,omitempty"`
	Path       string                `json:"path,omitempty"`
	Method     string                `json:"method,omitempty"`
	Network    gateway.NetworkPolicy `json:"network,omitempty"`
	Parameters map[string]Parameter  `json:"parameters"`
}
type Permission struct {
	ApprovedCases []string `json:"approved_cases"`
	Projects      []string `json:"projects,omitempty"`
	Certificate   string   `json:"certificate,omitempty"`
}
type PrincipalPolicy struct {
	Operations map[string]Permission `json:"operations"`
}
type Config struct {
	Mode           string                     `json:"-"`
	Operations     map[string]Operation       `json:"operations"`
	Principals     map[string]PrincipalPolicy `json:"principals"`
	Verify         gateway.Verifier           `json:"-"`
	certificateDir string
}
type Input struct {
	CaseID        string                     `json:"case_id"`
	Justification string                     `json:"justification"`
	Parameters    map[string]json.RawMessage `json:"parameters"`
}

func fixedPath(p string) bool {
	if p == "" || strings.ContainsAny(p, "?#%\\") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." || !shared.ValidID(part) {
			return false
		}
	}
	return true
}
func ConfigFromEnv() (Config, error) {
	var c Config
	if err := gateway.ReadConfig(os.Getenv("GATEWAY_CONFIG"), &c); err != nil {
		return c, err
	}
	c.Mode = os.Getenv("GATEWAY_MODE")
	token, err := gateway.ReadSecret(os.Getenv("GATEWAY_SERVICE_TOKEN_FILE"))
	if err != nil {
		return c, err
	}
	controller := os.Getenv("CONTROLLER_URL")
	if controller == "" {
		controller = "http://runtime-controller:8080"
	}
	c.Verify, err = gateway.NewVerifier(controller, token)
	return c, err
}
func NewHandler(c Config) (http.Handler, error) {
	if c.Verify == nil || (c.Mode != "fixture" && c.Mode != "upstream") || len(c.Operations) == 0 || len(c.Principals) == 0 {
		return nil, gateway.ErrDenied
	}
	endpoints := map[string]string{}
	for name, op := range c.Operations {
		if !shared.ValidID(name) {
			return nil, gateway.ErrDenied
		}
		for key, p := range op.Parameters {
			if !shared.ValidID(key) || (p.Type != "string" && p.Type != "number" && p.Type != "boolean") || p.MaxLength < 0 || p.MaxLength > 8192 {
				return nil, gateway.ErrDenied
			}
		}
		if c.Mode == "upstream" {
			u, err := gateway.HTTPSOrigin(op.Origin)
			if err != nil || !strings.HasPrefix(op.Path, "/") || !fixedPath(strings.TrimPrefix(op.Path, "/")) || (op.Method != "POST" && op.Method != "PUT" && op.Method != "PATCH") {
				return nil, gateway.ErrDenied
			}
			u.Path = op.Path
			endpoints[name] = u.String()
			tr, err := op.Network.Transport(nil)
			if err != nil {
				return nil, err
			}
			tr.CloseIdleConnections()
		}
	}
	for principal, policy := range c.Principals {
		if principal == "" {
			return nil, gateway.ErrDenied
		}
		for name, p := range policy.Operations {
			if _, ok := c.Operations[name]; !ok {
				return nil, gateway.ErrDenied
			}
			for _, caseID := range p.ApprovedCases {
				if !shared.ValidID(caseID) {
					return nil, gateway.ErrDenied
				}
			}
			for _, project := range p.Projects {
				if !shared.ValidID(project) {
					return nil, gateway.ErrDenied
				}
			}
			if c.Mode == "upstream" && (len(p.Projects) == 0 || !shared.ValidID(p.Certificate)) {
				return nil, gateway.ErrDenied
			}
		}
	}
	if c.certificateDir == "" {
		c.certificateDir = "/run/secrets"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { shared.JSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /v1/operations/{operation}", func(w http.ResponseWriter, r *http.Request) {
		id, err := gateway.Authenticate(r, c.Verify, "broker")
		if err != nil {
			shared.Error(w, 401, "UNAUTHORIZED")
			return
		}
		name := r.PathValue("operation")
		op, ok := c.Operations[name]
		policy, hasPrincipal := c.Principals[id.PrincipalID]
		permission, hasPermission := policy.Operations[name]
		if !ok || !hasPrincipal || !hasPermission {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if len(permission.Projects) > 0 && !slices.Contains(permission.Projects, id.ProjectID) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		b, err := gateway.ReadBounded(r.Body, 64<<10)
		if err != nil {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		var in Input
		if gateway.Decode(b, &in) != nil || !validInput(in, op) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if !slices.Contains(permission.ApprovedCases, in.CaseID) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if c.Mode == "fixture" {
			shared.JSON(w, 200, map[string]any{"mode": "fixture", "synthetic": true, "operation": name, "principal_id": id.PrincipalID, "workspace_id": id.WorkspaceID, "project_id": id.ProjectID, "case_id": in.CaseID, "parameters": in.Parameters})
			return
		}
		cert, err := loadCertificate(filepath.Join(c.certificateDir, permission.Certificate+".pem"))
		if err != nil {
			shared.Error(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		// Each request has its own certificate and connection pool. An identity can
		// never reuse a TLS session or connection authenticated as another identity.
		tr, err := op.Network.Transport(&cert)
		if err != nil {
			shared.Error(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		defer tr.CloseIdleConnections()
		client := &http.Client{Transport: tr, Timeout: 20 * time.Second, CheckRedirect: gateway.NoRedirect}
		payload, _ := json.Marshal(map[string]any{"case_id": in.CaseID, "justification": in.Justification, "parameters": in.Parameters, "principal_id": id.PrincipalID, "workspace_id": id.WorkspaceID, "project_id": id.ProjectID})
		req, err := http.NewRequestWithContext(r.Context(), op.Method, endpoints[name], bytes.NewReader(payload))
		if err != nil {
			shared.Error(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			shared.Error(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		defer res.Body.Close()
		response, err := gateway.ReadBounded(res.Body, 1<<20)
		if err != nil || res.StatusCode < 200 || res.StatusCode >= 300 || !json.Valid(response) {
			shared.Error(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(response)
	})
	return mux, nil
}
func validInput(in Input, op Operation) bool {
	if in.Parameters == nil || !shared.ValidID(in.CaseID) || strings.TrimSpace(in.Justification) == "" || len(in.Justification) > 2048 || len(in.Parameters) > 32 {
		return false
	}
	for key, p := range op.Parameters {
		if p.Required {
			if _, ok := in.Parameters[key]; !ok {
				return false
			}
		}
	}
	for key, raw := range in.Parameters {
		p, ok := op.Parameters[key]
		if !ok || len(raw) > 16384 {
			return false
		}
		if string(raw) == "null" {
			return false
		}
		switch p.Type {
		case "string":
			var v string
			if json.Unmarshal(raw, &v) != nil {
				return false
			}
			limit := p.MaxLength
			if limit == 0 {
				limit = 1024
			}
			if len(v) > limit {
				return false
			}
			if len(p.Enum) > 0 && !slices.Contains(p.Enum, v) {
				return false
			}
		case "number":
			var v float64
			if json.Unmarshal(raw, &v) != nil {
				return false
			}
		case "boolean":
			var v bool
			if json.Unmarshal(raw, &v) != nil {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func loadCertificate(path string) (tls.Certificate, error) {
	file, err := os.Open(path)
	if err != nil {
		return tls.Certificate{}, err
	}
	defer file.Close()
	pem, err := gateway.ReadBounded(file, 256<<10)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(pem, pem)
}
