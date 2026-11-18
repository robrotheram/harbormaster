package docker

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
)

type Docker struct {
	HTTP *http.Client
	Base string
}

func NewDocker(socket string) *Docker {
	t := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	return &Docker{HTTP: &http.Client{Transport: t, Timeout: 35 * time.Second}, Base: "http://docker/v1.47"}
}
func (d *Docker) call(ctx context.Context, method, path string, body any, out any) error {
	var b []byte
	var e error
	if body != nil {
		b, e = json.Marshal(body)
		if e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, d.Base+path, bytes.NewReader(b))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := d.HTTP.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return protocol.APIError{Status: resp.StatusCode}
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
	}
	return nil
}

type containerInfo struct {
	ID    string `json:"Id"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	Config struct {
		Env    []string          `json:"Env"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		Runtime string `json:"Runtime"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}
type ControllerConfig struct {
	EgressContainer                                                                                                                    string
	MemoryLimit                                                                                                                        int64
	Token, GatewayServiceToken, GrantSecret, Deployment, Image, RuntimeType, SelfContainer, InferenceContainer, BrokerContainer, Model string
	DefaultProvider, CodexModel, ClaudeModel                                                                                           string
	Console                                                                                                                            bool
	RequireVerified                                                                                                                    bool
}
type Controller struct {
	policies  sync.Map
	Docker    *Docker
	cfg       ControllerConfig
	mu        sync.Mutex
	agentHTTP *http.Client
}

func NewController(d *Docker, c ControllerConfig) (*Controller, error) {
	if len(c.Token) < 32 || len(c.GatewayServiceToken) < 32 || len(c.GrantSecret) < 32 || !shared.ValidID(c.Deployment) || c.Image == "" {
		return nil, errors.New("invalid controller configuration")
	}
	if c.RuntimeType != "" && c.RuntimeType != "docker" {
		return nil, errors.New("unsupported runtime")
	}
	if c.MemoryLimit == 0 {
		c.MemoryLimit = 512 << 20
	}
	return &Controller{Docker: d, cfg: c, agentHTTP: &http.Client{Timeout: 32 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Controller) name(id string) string { return c.cfg.Deployment + "-" + id }
func (c *Controller) labels(id string) map[string]string {
	return map[string]string{"workbench.deployment": c.cfg.Deployment, "workbench.workspace": id}
}
func (c *Controller) token(id, generation string) string {
	return protocol.Grant(c.cfg.GrantSecret, id, generation)
}
func (c *Controller) inspect(ctx context.Context, id string) (containerInfo, error) {
	var out containerInfo
	e := c.Docker.call(ctx, "GET", "/containers/"+c.name(id)+"/json", nil, &out)
	if e == nil && (out.Config.Labels["workbench.deployment"] != c.cfg.Deployment || out.Config.Labels["workbench.workspace"] != id || !protocol.HasOwnershipLabels(out.Config.Labels)) {
		return out, errors.New("unmanaged container")
	}
	return out, e
}
func (c *Controller) ensureNetwork(ctx context.Context, id string) error {
	info, err := c.inspect(ctx, id)
	if err != nil {
		return err
	}
	return c.ensureOwnedNetwork(ctx, id, protocol.WorkspaceSpec{ProjectID: info.Config.Labels["workbench.project"], HomeID: info.Config.Labels["workbench.home"]})
}
func (c *Controller) ensureOwnedNetwork(ctx context.Context, id string, spec protocol.WorkspaceSpec) error {
	name := c.name(id)
	var info struct {
		Labels   map[string]string `json:"Labels"`
		Internal bool              `json:"Internal"`
	}
	e := c.Docker.call(ctx, "GET", "/networks/"+name, nil, &info)
	if e != nil {
		var de protocol.APIError
		if !errors.As(e, &de) || de.Status != 404 {
			return e
		}
		e = c.Docker.call(ctx, "POST", "/networks/create", map[string]any{"Name": name, "Driver": "bridge", "Internal": true, "EnableIPv6": false, "Labels": c.ownedLabels(id, spec)}, nil)
		if e != nil {
			return e
		}
		info.Labels = c.ownedLabels(id, spec)
	} else if !info.Internal || info.Labels["workbench.workspace"] != id || info.Labels["workbench.deployment"] != c.cfg.Deployment {
		return errors.New("network ownership mismatch")
	}
	if protocol.CheckOwnership(info.Labels, spec) != nil && e == nil {
		return errors.New("network project ownership mismatch")
	}
	items := []struct{ Name, Alias string }{{c.cfg.SelfContainer, "runtime-controller"}, {c.cfg.InferenceContainer, "inference-gateway"}, {c.cfg.BrokerContainer, "credential-broker"}}
	if c.cfg.EgressContainer != "" {
		items = append(items, struct{ Name, Alias string }{c.cfg.EgressContainer, "egress-gateway"})
	}
	for _, item := range items {
		if item.Name == "" {
			return errors.New("platform container identity missing")
		}
		var platform containerInfo
		if e = c.Docker.call(ctx, "GET", "/containers/"+url.PathEscape(item.Name)+"/json", nil, &platform); e != nil {
			return e
		}
		if _, ok := platform.NetworkSettings.Networks[name]; ok {
			continue
		}
		if e = c.Docker.call(ctx, "POST", "/networks/"+name+"/connect", map[string]any{"Container": platform.ID, "EndpointConfig": map[string]any{"Aliases": []string{item.Alias}}}, nil); e != nil {
			return e
		}
	}
	return nil
}
func (c *Controller) ensure(ctx context.Context, id string, spec protocol.WorkspaceSpec) (containerInfo, error) {
	var err error
	spec, err = protocol.NormalizeSpec(spec)
	if err != nil {
		return containerInfo{}, err
	}
	if spec.AgentKind == "hermes" {
		return containerInfo{}, errors.New("unsupported agent kind")
	}
	if err := protocol.CheckResources(spec.Resources, protocol.ResourceLimits{CPUCores: 1, MemoryBytes: c.cfg.MemoryLimit}); err != nil {
		return containerInfo{}, err
	}
	info, e := c.inspect(ctx, id)
	if e == nil {
		if info.HostConfig.Runtime != "runc" {
			return info, errors.New("existing workspace runtime mismatch")
		}
		if protocol.CheckOwnership(info.Config.Labels, spec) != nil {
			return info, errors.New("container project ownership mismatch")
		}
		if kind := info.Config.Labels["workbench.agent"]; kind != "" && kind != spec.AgentKind {
			return info, errors.New("agent kind mismatch")
		}
		if e = c.ensureOwnedNetwork(ctx, id, spec); e != nil {
			return info, e
		}

		// Replace stopped containers when their configuration requires recreation.
		// Never replace a running container or request Docker volume removal.
		if !info.State.Running && (legacyToolSelection(info) || (connectionMode(info) == "open") != (spec.ConnectionMode == "open")) {
			if e = c.Docker.call(ctx, "DELETE", "/containers/"+info.ID, nil, nil); e != nil {
				return info, e
			}
			return c.ensure(ctx, id, spec)
		}
		return info, nil
	}
	var de protocol.APIError
	if !errors.As(e, &de) || de.Status != 404 {
		return info, e
	}
	if e = c.ensureOwnedNetwork(ctx, id, spec); e != nil {
		return info, e
	}
	volume := c.name(id) + "-data"
	var vol struct {
		Labels map[string]string `json:"Labels"`
	}
	e = c.Docker.call(ctx, "GET", "/volumes/"+volume, nil, &vol)
	if e != nil {
		if !errors.As(e, &de) || de.Status != 404 {
			return info, e
		}
		if e = c.Docker.call(ctx, "POST", "/volumes/create", map[string]any{"Name": volume, "Labels": c.ownedLabels(id, spec)}, nil); e != nil {
			return info, e
		}
		vol.Labels = c.ownedLabels(id, spec)
	} else if vol.Labels["workbench.workspace"] != id || vol.Labels["workbench.deployment"] != c.cfg.Deployment {
		return info, errors.New("volume ownership mismatch")
	}
	if e == nil && protocol.CheckOwnership(vol.Labels, spec) != nil {
		return info, errors.New("volume project ownership mismatch")
	}
	generation := shared.NewID("gen_")
	labels := c.ownedLabels(id, spec)
	labels["workbench.agent"], labels["workbench.template"] = spec.AgentKind, spec.TemplateID
	labels["workbench.generation"] = generation
	labels["workbench.parent"] = spec.ParentID
	body := map[string]any{
		"Image": c.cfg.Image, "User": "10001:10001", "Labels": labels,
		"Env":              []string{"PROJECT_ID=" + spec.ProjectID, "AGENT_HOME_ID=" + spec.HomeID, "AGENT_KIND=" + spec.AgentKind, "WORKSPACE_ID=" + id, "WORKSPACE_TOKEN=" + c.token(id, generation), "INFERENCE_URL=http://inference-gateway:8080", "BROKER_URL=http://credential-broker:8080", "INFERENCE_MODEL=" + c.cfg.Model, "WORKSPACE_AGENT=" + c.cfg.DefaultProvider, "CODEX_MODEL=" + c.cfg.CodexModel, "CLAUDE_MODEL=" + c.cfg.ClaudeModel, "MONITOR_PROCESSES=true", "WORKSPACE_T3=" + fmt.Sprint(spec.ConsoleEnabled || c.cfg.Console && spec.AgentKind == "t3"), "WORKSPACE_BRANCH=" + spec.Branch, "WORKSPACE_NAME=" + spec.Name},
		"HostConfig":       map[string]any{"Init": true, "Runtime": "runc", "NetworkMode": c.name(id), "Dns": []string{"127.0.0.1"}, "ReadonlyRootfs": true, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "Memory": c.cfg.MemoryLimit, "MemorySwap": c.cfg.MemoryLimit, "NanoCpus": int64(1e9), "PidsLimit": 128, "Tmpfs": map[string]string{"/tmp": "rw,nosuid,nodev,noexec,size=64m"}, "Mounts": []any{map[string]any{"Type": "volume", "Source": volume, "Target": "/data"}}, "LogConfig": map[string]any{"Type": "none"}},
		"NetworkingConfig": map[string]any{"EndpointsConfig": map[string]any{c.name(id): map[string]any{"Aliases": []string{"workspace"}}}},
	}
	if c.cfg.EgressContainer != "" {
		body["Env"] = append(body["Env"].([]string), "EGRESS_URL=http://egress-gateway:8080")
	}
	body["Env"] = append(body["Env"].([]string), "WORKSPACE_CONNECTION_MODE="+spec.ConnectionMode)
	if spec.ConnectionMode == "open" {
		network, err := c.ensureInternetNetwork(ctx, id, spec)
		if err != nil {
			return info, err
		}
		body["HostConfig"].(map[string]any)["Dns"] = []string{}
		body["NetworkingConfig"].(map[string]any)["EndpointsConfig"].(map[string]any)[network] = map[string]any{}
	}
	if e = c.Docker.call(ctx, "POST", "/containers/create?name="+c.name(id), body, nil); e != nil {
		return info, e
	}
	return c.inspect(ctx, id)
}
func (c *Controller) agent(ctx context.Context, info containerInfo, method, path string, b []byte) (int, []byte, error) {
	id := info.Config.Labels["workbench.workspace"]
	endpoint, ok := info.NetworkSettings.Networks[c.name(id)]
	if !ok || net.ParseIP(endpoint.IPAddress) == nil {
		return 0, nil, errors.New("workspace address unavailable")
	}
	req, e := http.NewRequestWithContext(ctx, method, "http://"+net.JoinHostPort(endpoint.IPAddress, "8080")+path, bytes.NewReader(b))
	if e != nil {
		return 0, nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token(id, info.Config.Labels["workbench.generation"]))
	resp, e := c.agentHTTP.Do(req)
	if e != nil {
		return 0, nil, e
	}
	defer resp.Body.Close()
	out, e := io.ReadAll(io.LimitReader(resp.Body, protocol.AgentResponseLimit(path)+1))
	if int64(len(out)) > protocol.AgentResponseLimit(path) {
		return 0, nil, errors.New("workspace response too large")
	}
	return resp.StatusCode, out, e
}
func (c *Controller) observe(ctx context.Context, id string) (shared.Observation, error) {
	o := shared.Observation{RuntimeType: "docker", IsolationStatus: "unverified", NetworkStatus: "unverified"}
	info, e := c.inspect(ctx, id)
	if e != nil {
		var de protocol.APIError
		if errors.As(e, &de) && de.Status == 404 {
			return o, nil
		}
		return o, e
	}
	o.Running = info.State.Running
	o.ConnectionMode = connectionMode(info)
	if info.HostConfig.Runtime == "io.containerd.kata.v2" {
		o.RuntimeType = "kata"
	}
	if !o.Running {
		return o, nil
	}
	// Existing unverified containers must not bypass a newly enabled secure gate.
	if c.cfg.RequireVerified {
		return o, nil
	}
	status, b, e := c.agent(ctx, info, "GET", "/v1/activity", nil)
	if e == nil && status == 200 && json.Unmarshal(b, &o.Activity) == nil {
		o.Ready = true
	}
	// Metrics describe the complete container, including guest/runtime overhead.
	var stats struct {
		Memory struct {
			Usage int64 `json:"usage"`
		} `json:"memory_stats"`
		CPU struct {
			Usage struct {
				Total uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			System uint64 `json:"system_cpu_usage"`
			Online uint64 `json:"online_cpus"`
		} `json:"cpu_stats"`
		Previous struct {
			Usage struct {
				Total uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			System uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
	}
	metricsCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if c.Docker.call(metricsCtx, "GET", "/containers/"+info.ID+"/stats?stream=false&one-shot=true", nil, &stats) == nil {
		mem := stats.Memory.Usage
		o.MemoryBytes = &mem
		if stats.CPU.System > stats.Previous.System && stats.CPU.Usage.Total >= stats.Previous.Usage.Total && stats.CPU.Online > 0 {
			v := float64(stats.CPU.Usage.Total-stats.Previous.Usage.Total) / float64(stats.CPU.System-stats.Previous.System) * float64(stats.CPU.Online)
			o.CPUCores = &v
		}
	}
	return o, nil
}

func (c *Controller) Handler() http.Handler {
	return protocol.NewControllerHandler(protocol.Backend{
		Token: c.cfg.Token, GatewayServiceToken: c.cfg.GatewayServiceToken,
		Health: c.health, VerifyGrant: c.verify,
		Start: c.start, Stop: c.stop, Inspect: c.inspectWorkspace,
		BindNetworkPolicy: protocol.BindNetworkPolicy(c.applyNetwork), Reconcile: c.reconcileNetwork, Capabilities: c.capabilities,
		Proxy: c.proxy, Agent: protocol.BufferedAgent(c.proxyAgent),
		StartBodyLimit: 2048, AllowEmptyStart: true, ValidateAgentID: true, AgentNoStore: true,
	})
}

func (c *Controller) health(w http.ResponseWriter, r *http.Request) {
	if c.Docker.call(r.Context(), "GET", "/_ping", nil, nil) != nil {
		shared.Error(w, 503, "DOCKER_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, map[string]string{"status": "ok"})
}

func (c *Controller) capabilities() protocol.RuntimeCapability {
	caps := RuntimeCapabilities()
	caps.Resources.MemoryBytes = c.cfg.MemoryLimit
	return caps
}

func (c *Controller) applyNetwork(ctx context.Context, id string, connections *shared.ConnectionPolicy) *protocol.ControllerError {
	if !shared.ValidID(id) || c.cfg.RequireVerified {
		return protocol.Failure(503, "NETWORK_POLICY_UNAVAILABLE")
	}
	if connections == nil || !connections.Valid() {
		return protocol.Failure(400, "INVALID_REQUEST")
	}
	info, err := c.inspect(ctx, id)
	if err != nil || !info.State.Running {
		return protocol.Failure(409, "NOT_READY")
	}
	if (connections.Mode == "open") != (connectionMode(info) == "open") {
		return protocol.Failure(409, "STOP_WORKSPACE_FIRST")
	}
	c.policies.Store(info.Config.Labels["workbench.generation"], *connections)
	return nil
}

func (c *Controller) reconcileNetwork(ctx context.Context, id string) *protocol.ControllerError {
	if !shared.ValidID(id) || c.cfg.RequireVerified {
		return protocol.Failure(403, "NOT_FOUND_OR_FORBIDDEN")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	info, err := c.inspect(ctx, id)
	if err != nil || !info.State.Running {
		return protocol.Failure(409, "NOT_READY")
	}
	if c.ensureNetwork(ctx, id) != nil {
		return protocol.Failure(503, "NETWORK_UNAVAILABLE")
	}
	return nil
}

func (c *Controller) inspectWorkspace(ctx context.Context, id string) (shared.Observation, *protocol.ControllerError) {
	if !shared.ValidID(id) {
		return shared.Observation{}, protocol.Failure(400, "INVALID_REQUEST")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	o, e := c.observe(ctx, id)
	if e != nil {
		return shared.Observation{}, protocol.Failure(503, "RUNTIME_UNAVAILABLE")
	}
	return o, nil
}

func (c *Controller) stop(ctx context.Context, id string) *protocol.ControllerError {
	if !shared.ValidID(id) {
		return protocol.Failure(400, "INVALID_REQUEST")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	info, e := c.inspect(ctx, id)
	var de protocol.APIError
	if errors.As(e, &de) && de.Status == 404 {
		return nil
	}
	if e != nil {
		return protocol.Failure(503, "RUNTIME_UNAVAILABLE")
	}
	if info.State.Running {
		status, _, e := c.agent(ctx, info, "POST", "/v1/quiesce", nil)
		if e != nil || status != 200 {
			return protocol.Failure(409, "WORKSPACE_BUSY")
		}
		if e = c.Docker.call(ctx, "POST", "/containers/"+info.ID+"/stop?t=5", nil, nil); e != nil {
			_, _, _ = c.agent(ctx, info, "POST", "/v1/resume", nil)
			return protocol.Failure(503, "RUNTIME_UNAVAILABLE")
		}
	}
	return nil
}

func (c *Controller) start(ctx context.Context, id string, spec protocol.WorkspaceSpec) (shared.Observation, *protocol.ControllerError) {
	if _, err := protocol.NormalizeSpec(spec); err != nil {
		return shared.Observation{}, protocol.Failure(400, "UNSUPPORTED_WORKSPACE_SPEC")
	}
	if (spec.ParentID != "" && !shared.ValidID(spec.ParentID)) || (spec.Branch != "" && !shared.ValidBranch(spec.Branch)) {
		return shared.Observation{}, protocol.Failure(400, "INVALID_REQUEST")
	}
	if !shared.ValidID(id) {
		return shared.Observation{}, protocol.Failure(400, "INVALID_REQUEST")
	}
	if c.cfg.RequireVerified {
		return shared.Observation{}, protocol.Failure(503, "VERIFIED_ISOLATION_UNAVAILABLE")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	info, e := c.ensure(ctx, id, spec)
	if e != nil {
		return shared.Observation{}, protocol.Failure(503, "RUNTIME_UNAVAILABLE")
	}
	if spec.ParentID != "" {
		if err := c.seedFromParent(ctx, info, spec.ParentID); err != nil {
			return shared.Observation{}, protocol.Failure(503, "WORKSPACE_SEED_UNAVAILABLE")
		}
	}
	if !info.State.Running {
		if e = c.Docker.call(ctx, "POST", "/containers/"+info.ID+"/start", nil, nil); e != nil {
			return shared.Observation{}, protocol.Failure(503, "RUNTIME_UNAVAILABLE")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	for {
		info, e = c.inspect(ctx, id)
		if e == nil {
			status, _, err := c.agent(ctx, info, "POST", "/v1/resume", nil)
			if err == nil && status == 200 {
				break
			}
		}
		select {
		case <-ctx.Done():
			return shared.Observation{}, protocol.Failure(503, "NOT_READY")
		case <-time.After(200 * time.Millisecond):
		}
	}
	o, e := c.observe(ctx, id)
	if e != nil {
		return shared.Observation{}, protocol.Failure(503, "RUNTIME_UNAVAILABLE")
	}
	return o, nil
}

func (c *Controller) proxy(w http.ResponseWriter, r *http.Request, id, path, prefix string) {
	if !shared.ValidID(id) || c.cfg.RequireVerified {
		shared.Error(w, 403, "CONSOLE_UNAVAILABLE")
		return
	}
	info, err := c.inspect(r.Context(), id)
	if err != nil || !info.State.Running {
		shared.Error(w, 503, "CONSOLE_NOT_READY")
		return
	}
	endpoint := info.NetworkSettings.Networks[c.name(id)]
	if net.ParseIP(endpoint.IPAddress) == nil {
		shared.Error(w, 503, "CONSOLE_NOT_READY")
		return
	}
	shared.Proxy(w, r, "http://"+net.JoinHostPort(endpoint.IPAddress, "8080"), prefix+path, c.token(id, info.Config.Labels["workbench.generation"]))
}

func (c *Controller) proxyAgent(ctx context.Context, id, method, path string, body []byte) (int, []byte, *protocol.ControllerError) {
	c.mu.Lock()
	info, e := c.inspect(ctx, id)
	c.mu.Unlock()
	if e != nil || !info.State.Running {
		return 0, nil, protocol.Failure(503, "NOT_READY")
	}
	status, b, e := c.agent(ctx, info, method, path, body)
	if e != nil {
		return 0, nil, protocol.Failure(503, "WORKSPACE_UNAVAILABLE")
	}
	return status, b, nil
}

func (c *Controller) verify(w http.ResponseWriter, r *http.Request) {
	if c.cfg.RequireVerified {
		shared.Error(w, 503, "VERIFIED_ISOLATION_UNAVAILABLE")
		return
	}
	var req struct {
		Token    string `json:"token"`
		Audience string `json:"audience"`
	}
	if !shared.Decode(w, r, &req, 2048) {
		return
	}
	if len(req.Token) != 64 || (req.Audience != "inference" && req.Audience != "broker" && req.Audience != "egress") {
		shared.Error(w, 403, "GRANT_REJECTED")
		return
	}
	filters, _ := json.Marshal(map[string]any{"label": []string{"workbench.deployment=" + c.cfg.Deployment}})
	var containers []struct {
		ID     string            `json:"Id"`
		Labels map[string]string `json:"Labels"`
	}
	if c.Docker.call(r.Context(), "GET", "/containers/json?filters="+url.QueryEscape(string(filters)), nil, &containers) != nil {
		shared.Error(w, 503, "GRANT_VERIFIER_UNAVAILABLE")
		return
	}
	for _, info := range containers {
		id := info.Labels["workbench.workspace"]
		generation := info.Labels["workbench.generation"]
		if info.Labels["workbench.deployment"] == c.cfg.Deployment && protocol.HasOwnershipLabels(info.Labels) && shared.ValidID(id) && generation != "" && subtle.ConstantTimeCompare([]byte(c.token(id, generation)), []byte(req.Token)) == 1 {
			identity := map[string]any{"workspace_id": id, "project_id": info.Labels["workbench.project"], "principal_id": "local-operator"}
			if req.Audience == "egress" {
				v, _ := c.policies.Load(generation)
				policy, ok := v.(shared.ConnectionPolicy)
				if !ok || policy.Mode == "blocked" {
					shared.Error(w, 403, "GRANT_REJECTED")
					return
				}
				identity["connections"] = policy
			}
			shared.JSON(w, 200, identity)
			return
		}
	}
	shared.Error(w, 403, "GRANT_REJECTED")
}

// Presence marks a stopped container built with the retired selectable-tool
// configuration. Running containers are preserved; replacement keeps its volume.
func legacyToolSelection(info containerInfo) bool {
	return slices.ContainsFunc(info.Config.Env, func(value string) bool {
		return strings.HasPrefix(value, "WORKSPACE_MCPS=")
	})
}
