package docker

import (
	"agent-workbench/internal/shared"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fakeEngine struct {
	mu               sync.Mutex
	created, running bool
	createCount      int
	removed          int
	spec             map[string]any
	runtime, network string
	failStart        bool
}

func (f *fakeEngine) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/v1.47")
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == "GET" && strings.HasPrefix(path, "/networks/"):
		http.Error(w, "missing", 404)
	case r.Method == "POST" && path == "/networks/create":
		var b struct {
			Name     string
			Internal bool
		}
		json.NewDecoder(r.Body).Decode(&b)
		if b.Internal {
			f.network = b.Name
		}
		io.WriteString(w, `{"Id":"network"}`)
	case r.Method == "POST" && strings.HasSuffix(path, "/connect"):
		w.WriteHeader(200)
	case r.Method == "GET" && strings.HasPrefix(path, "/volumes/"):
		http.Error(w, "missing", 404)
	case r.Method == "POST" && path == "/volumes/create":
		io.WriteString(w, `{}`)
	case r.Method == "DELETE" && strings.HasPrefix(path, "/containers/"):
		if f.running || r.URL.RawQuery != "" {
			http.Error(w, "unsafe removal", 409)
			return
		}
		f.created = false
		f.removed++
		w.WriteHeader(204)
	case r.Method == "POST" && path == "/containers/create":
		f.created = true
		f.createCount++
		json.NewDecoder(r.Body).Decode(&f.spec)
		f.runtime = f.spec["HostConfig"].(map[string]any)["Runtime"].(string)
		w.WriteHeader(201)
		io.WriteString(w, `{"Id":"container"}`)
	case r.Method == "POST" && strings.HasSuffix(path, "/start"):
		if f.failStart {
			http.Error(w, "runtime unavailable", 500)
			return
		}
		f.running = true
		w.WriteHeader(204)
	case r.Method == "POST" && strings.HasSuffix(path, "/stop"):
		f.running = false
		w.WriteHeader(204)
	case r.Method == "GET" && path == "/containers/json":
		if !f.running {
			io.WriteString(w, `[]`)
			return
		}
		shared.JSON(w, 200, []any{map[string]any{"Id": "container", "Labels": f.spec["Labels"]}})
	case r.Method == "GET" && strings.Contains(path, "/stats"):
		io.WriteString(w, `{"memory_stats":{"usage":1024}}`)
	case r.Method == "GET" && strings.HasSuffix(path, "/json"):
		if path == "/containers/self/json" || path == "/containers/inference/json" || path == "/containers/broker/json" || path == "/containers/egress/json" {
			shared.JSON(w, 200, map[string]any{"Id": strings.Split(path, "/")[2], "NetworkSettings": map[string]any{"Networks": map[string]any{}}})
			return
		}
		if !f.created {
			http.Error(w, "missing", 404)
			return
		}
		shared.JSON(w, 200, map[string]any{"Id": "container", "State": map[string]bool{"Running": f.running}, "Config": map[string]any{"Labels": f.spec["Labels"], "Env": f.spec["Env"]}, "HostConfig": map[string]string{"Runtime": f.runtime}, "NetworkSettings": map[string]any{"Networks": map[string]any{f.network: map[string]string{"IPAddress": "172.30.0.9"}}}})
	default:
		http.Error(w, "unexpected", 500)
	}
}
func controller(t *testing.T, require bool) (*Controller, *fakeEngine) {
	t.Helper()
	engine := &fakeEngine{}
	server := httptest.NewServer(http.HandlerFunc(engine.handler))
	t.Cleanup(server.Close)
	cfg := ControllerConfig{Token: strings.Repeat("c", 32), GatewayServiceToken: strings.Repeat("g", 32), GrantSecret: strings.Repeat("s", 32), Deployment: "testawb", Image: "fixed-workspace:dev", RuntimeType: "docker", SelfContainer: "self", InferenceContainer: "inference", BrokerContainer: "broker", Model: "fixture", RequireVerified: require}
	c, e := NewController(&Docker{HTTP: server.Client(), Base: server.URL + "/v1.47"}, cfg)
	if e != nil {
		t.Fatal(e)
	}
	c.agentHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		if r.URL.Path == "/v1/activity" {
			b, _ := json.Marshal(shared.Activity{Known: true, LastActivity: time.Now().UTC()})
			body = string(b)
		} else {
			body = `{"ready":true}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	return c, engine
}
func controllerRequest(c *Controller, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	c.Handler().ServeHTTP(w, r)
	return w
}
func TestFixedExecutionProfileAndGrantRevocation(t *testing.T) {
	c, engine := controller(t, false)
	if r := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", "", ""); r.Code != 401 {
		t.Fatal("unauthorized start")
	}
	if r := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, `{}`); r.Code != 200 {
		t.Fatalf("start: %d %s", r.Code, r.Body)
	}
	host := engine.spec["HostConfig"].(map[string]any)
	if host["Runtime"] != "runc" || host["ReadonlyRootfs"] != true || host["NetworkMode"] != "testawb-ws_test" || host["Init"] != true {
		t.Fatal("fixed execution profile lost")
	}
	if engine.spec["Image"] != "fixed-workspace:dev" {
		t.Fatal("image override accepted")
	}
	if host["Privileged"] == true || host["Binds"] != nil {
		t.Fatal("host access exposed")
	}
	if !bytes.Contains(mustJSON(host["Dns"]), []byte("127.0.0.1")) {
		t.Fatal("external DNS fallback enabled")
	}
	labels := engine.spec["Labels"].(map[string]any)
	token := c.token("ws_test", labels["workbench.generation"].(string))
	body := `{"token":"` + token + `","audience":"inference"}`
	if r := controllerRequest(c, "POST", "/v1/grants/verify", c.cfg.GatewayServiceToken, body); r.Code != 200 || !strings.Contains(r.Body.String(), "local-operator") {
		t.Fatal("grant verification failed")
	}
	if r := controllerRequest(c, "POST", "/v1/workspaces/ws_test/stop", c.cfg.Token, ""); r.Code != 200 {
		t.Fatal("stop failed")
	}
	if r := controllerRequest(c, "POST", "/v1/grants/verify", c.cfg.GatewayServiceToken, body); r.Code != 403 {
		t.Fatal("stopped workspace retained authority")
	}
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func TestVMRequiredModeFailsBeforeAllocation(t *testing.T) {
	c, engine := controller(t, true)
	r := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, "")
	if r.Code != 503 || engine.created {
		t.Fatal("verified mode fell back to ordinary Docker")
	}
}
func TestSecureGateAlsoBlocksExistingRuntime(t *testing.T) {
	c, engine := controller(t, false)
	if r := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, ""); r.Code != 200 {
		t.Fatal("setup failed")
	}
	c.cfg.RequireVerified = true
	r := controllerRequest(c, "GET", "/v1/workspaces/ws_test", c.cfg.Token, "")
	var observed shared.Observation
	if json.Unmarshal(r.Body.Bytes(), &observed) != nil || observed.Ready {
		t.Fatal("existing runtime bypassed secure mode")
	}
	labels := engine.spec["Labels"].(map[string]any)
	token := c.token("ws_test", labels["workbench.generation"].(string))
	r = controllerRequest(c, "POST", "/v1/grants/verify", c.cfg.GatewayServiceToken, `{"token":"`+token+`","audience":"inference"}`)
	if r.Code != 503 {
		t.Fatal("existing grant bypassed secure mode")
	}
}
func TestKataFailureDoesNotFallback(t *testing.T) {
	c, engine := controller(t, false)
	c.cfg.RuntimeType = "kata"
	engine.failStart = true
	r := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, "")
	if r.Code != 503 || engine.createCount != 1 || engine.runtime != "io.containerd.kata.v2" {
		t.Fatal("Kata fallback or invalid runtime selection")
	}
}
func TestAgentProxyCannotTargetManagement(t *testing.T) {
	c, _ := controller(t, false)
	for _, path := range []string{"/v1/workspaces/ws_test/agent/v1/quiesce", "/v1/workspaces/ws_test/agent/containers/json", "/v1/workspaces/ws_test/agent/http:evil.example"} {
		r := controllerRequest(c, "GET", path, c.cfg.Token, "")
		if r.Code != 404 {
			t.Fatalf("unbounded proxy path %s: %d", path, r.Code)
		}
	}
	if _, e := NewController(c.Docker, ControllerConfig{}); e == nil {
		t.Fatal("empty credentials accepted")
	}
}

func TestOpenNetworkIsExplicitAndRestrictiveTransitionNeedsStop(t *testing.T) {
	c, engine := controller(t, false)
	c.cfg.EgressContainer = "egress"
	r := controllerRequest(c, "POST", "/v1/workspaces/ws_open/start", c.cfg.Token, `{"connection_mode":"open"}`)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body)
	}
	host := engine.spec["HostConfig"].(map[string]any)
	if len(host["Dns"].([]any)) != 0 {
		t.Fatal("open profile still blocks DNS")
	}
	endpoints := engine.spec["NetworkingConfig"].(map[string]any)["EndpointsConfig"].(map[string]any)
	if len(endpoints) != 2 || endpoints["testawb-ws_open-internet"] == nil {
		t.Fatal("open network not attached")
	}
	r = controllerRequest(c, "POST", "/v1/workspaces/ws_open/network-policy", c.cfg.Token, `{"connections":{"mode":"blocked","destinations":[]}}`)
	if r.Code != 409 {
		t.Fatal("claimed restriction while direct internet still attached")
	}
	r = controllerRequest(c, "POST", "/v1/workspaces/ws_open/network-policy", c.cfg.Token, `{"connections":{"mode":"open","destinations":[]}}`)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body)
	}
}
