package controllerclient

import (
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	URL, Token string
	HTTP       *http.Client
}

var _ protocol.Runtime = (*Client)(nil)

func NewClient(url, token string) *Client {
	return &Client{strings.TrimRight(url, "/"), token, &http.Client{Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) Capabilities(ctx context.Context) (protocol.RuntimeCapability, error) {
	out, err := c.capabilities(ctx)
	if err == nil && !validCapabilities(out) {
		err = protocol.ErrCapabilitiesIncompatible
	}
	return out, err
}

func validCapabilities(out protocol.RuntimeCapability) bool {
	return out.SchemaVersion == 1 && out.NetworkModes != nil && out.ID != "" && out.Version != "" && len(out.AgentKinds) > 0 && out.Resources.CPUCores > 0 && out.Resources.MemoryBytes > 0
}

func (c *Client) capabilities(ctx context.Context) (protocol.RuntimeCapability, error) {
	var out protocol.RuntimeCapability
	s, b, e := c.do(ctx, "GET", "/v1/capabilities", nil)
	if e != nil {
		return out, e
	}
	if s != 200 {
		return out, fmt.Errorf("runtime capabilities unavailable: %d", s)
	}
	e = json.Unmarshal(b, &out)
	return out, e
}

func (c *Client) MatrixCredential(ctx context.Context, project string, config shared.HermesMatrix, token string) error {
	b, _ := json.Marshal(map[string]any{"config": config, "access_token": token})
	status, _, err := c.do(ctx, "POST", "/v1/projects/"+project+"/matrix-credential", b)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("Matrix credential validation refused: %d", status)
	}
	return nil
}

func (c *Client) MatrixAction(ctx context.Context, id, action string, config *shared.HermesMatrix) error {
	b, _ := json.Marshal(config)
	status, body, err := c.do(ctx, "POST", "/v1/workspaces/"+id+"/matrix-"+action, b)
	if err != nil {
		return err
	}
	if status != 200 {
		var e shared.APIError
		if json.Unmarshal(body, &e) == nil && e.Code != "" {
			return shared.CodedError{Status: status, Code: e.Code}
		}
		return fmt.Errorf("Matrix operation refused: %d", status)
	}
	return nil
}
func (c *Client) MatrixCredentialPresent(ctx context.Context, id string) bool {
	status, b, err := c.do(ctx, "GET", "/v1/projects/"+id+"/matrix-credential", nil)
	var out struct {
		Configured bool `json:"configured"`
	}
	return err == nil && status == 200 && json.Unmarshal(b, &out) == nil && out.Configured
}
func (c *Client) do(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	r, e := http.NewRequestWithContext(ctx, method, c.URL+path, bytes.NewReader(body))
	if e != nil {
		return 0, nil, e
	}
	r.Header.Set("Authorization", "Bearer "+c.Token)
	r.Header.Set("Content-Type", "application/json")
	resp, e := c.HTTP.Do(r)
	if e != nil {
		return 0, nil, e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, protocol.AgentResponseLimit(path)+1))
	if int64(len(b)) > protocol.AgentResponseLimit(path) {
		return 0, nil, fmt.Errorf("response too large")
	}
	return resp.StatusCode, b, e
}
func (c *Client) StartConfigured(ctx context.Context, id string, spec protocol.WorkspaceSpec) (shared.Observation, error) {
	var o shared.Observation
	body, _ := json.Marshal(spec)
	s, b, e := c.do(ctx, "POST", "/v1/workspaces/"+id+"/start", body)
	if e != nil {
		return o, e
	}
	if s != 200 {
		var failure shared.APIError
		if s == http.StatusServiceUnavailable && json.Unmarshal(b, &failure) == nil && failure.Code == protocol.DockerAddressPoolExhausted {
			return o, shared.CodedError{Status: s, Code: failure.Code}
		}
		return o, fmt.Errorf("runtime start refused: %d", s)
	}
	e = json.Unmarshal(b, &o)
	return o, e
}

func (c *Client) Console(w http.ResponseWriter, r *http.Request, id, path string) {
	shared.Proxy(w, r, c.URL, "/v1/workspaces/"+id+"/console/"+strings.TrimPrefix(path, "/"), c.Token)
}
func (c *Client) Desktop(w http.ResponseWriter, r *http.Request, id, path string) {
	shared.Proxy(w, r, c.URL, "/v1/workspaces/"+id+"/desktop/"+strings.TrimPrefix(path, "/"), c.Token)
}
func (c *Client) Reconcile(ctx context.Context, id string) error {
	status, _, err := c.do(ctx, "POST", "/v1/workspaces/"+id+"/reconcile-network", nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("network reconciliation refused: %d", status)
	}
	return nil
}
func (c *Client) Stop(ctx context.Context, id string) error {
	return c.stop(ctx, id, "stop")
}

func (c *Client) ForceStop(ctx context.Context, id string) error {
	return c.stop(ctx, id, "force-stop")
}

func (c *Client) stop(ctx context.Context, id, action string) error {
	s, _, e := c.do(ctx, "POST", "/v1/workspaces/"+id+"/"+action, nil)
	if e != nil {
		return e
	}
	if s != 200 {
		return fmt.Errorf("runtime stop refused: %d", s)
	}
	return nil
}
func (c *Client) Inspect(ctx context.Context, id string) (shared.Observation, error) {
	var o shared.Observation
	s, b, e := c.do(ctx, "GET", "/v1/workspaces/"+id, nil)
	if e != nil {
		return o, e
	}
	if s != 200 {
		return o, fmt.Errorf("runtime inspect refused: %d", s)
	}
	e = json.Unmarshal(b, &o)
	return o, e
}
func (c *Client) Agent(ctx context.Context, id, method, path string, b []byte) (int, []byte, error) {
	return c.do(ctx, method, "/v1/workspaces/"+id+"/agent"+path, b)
}

func (c *Client) ConfigureConnections(ctx context.Context, id string, policy shared.ConnectionPolicy) error {
	body, _ := json.Marshal(map[string]any{"connections": policy})
	status, _, err := c.do(ctx, "POST", "/v1/workspaces/"+id+"/network-policy", body)
	if err != nil {
		return err
	}
	if status == 409 {
		return shared.CodedError{Status: http.StatusConflict, Code: "STOP_WORKSPACE_FIRST"}
	}
	if status != 200 {
		return shared.CodedError{Status: http.StatusServiceUnavailable, Code: "NETWORK_POLICY_UNAVAILABLE"}
	}
	return nil
}

func (c *Client) Application(w http.ResponseWriter, r *http.Request, id, path string) {
	shared.Proxy(w, r, c.URL, "/v1/workspaces/"+id+"/application-proxy/"+strings.TrimPrefix(path, "/"), c.Token)
}

func (c *Client) EnsureProjectNamespace(ctx context.Context, project string) (string, error) {
	expected := shared.ProjectNamespace(project)
	if expected == "" {
		return "", fmt.Errorf("invalid deployment project")
	}
	status, body, err := c.do(ctx, "POST", "/v1/projects/"+project+"/namespace", []byte(`{}`))
	if err != nil {
		return "", err
	}
	var out struct {
		Namespace string `json:"namespace"`
	}
	if status != 200 || json.Unmarshal(body, &out) != nil || out.Namespace != expected {
		return "", fmt.Errorf("project namespace unavailable")
	}
	return out.Namespace, nil
}
