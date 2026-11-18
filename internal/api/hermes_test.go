package api

import (
	"agent-workbench/internal/runtime/controllerclient"
	runtime "agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHermesStatusThroughControllerClient(t *testing.T) {
	s, _ := setup(t)
	p := domainProject(t, s, "hermes-controller-status")
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/capabilities" || r.Header.Get("Authorization") != "Bearer management-token" {
			t.Error("wrong runtime probe")
		}
		w.Write([]byte(`{"schema_version":1,"resources":{"cpu_cores":1,"memory_bytes":1073741824},"network_modes":["blocked","allowlist"],"id":"development-v1","version":"firecracker-2","agent_kinds":["t3","codex","hermes"],"isolation_status":"jailed-vm","network_status":"no-nic-vsock","private_homes":true}`))
	}))
	defer controller.Close()
	s.Runtime = controllerclient.NewClient(controller.URL, "management-token")
	c := shared.HermesSetup{ProjectID: p.ID, Provider: "openai-codex", Mode: "workspace", Model: "approved-model"}
	body, _ := json.Marshal(c)
	path := "/v1/projects/" + p.ID + "/hermes"
	if w := request(t, s, "POST", path, string(body)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := request(t, s, "GET", path, "")
	var out struct {
		Config    shared.HermesSetup `json:"config"`
		Status    string             `json:"runtime_status"`
		Available bool               `json:"runtime_available"`
		Saved     bool               `json:"saved"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Status != "available" || !out.Available || !out.Saved || out.Config.Model != c.Model || out.Config.Revision != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
}

type capabilityRuntime struct {
	*embeddedRuntime
	cap runtime.RuntimeCapability
	err error
}

func (r *capabilityRuntime) Capabilities(context.Context) (runtime.RuntimeCapability, error) {
	return r.cap, r.err
}

func TestHermesRuntimeCheckFailureIsNotMissingInstallation(t *testing.T) {
	s, rt := setup(t)
	p := domainProject(t, s, "hermes-status")
	c := &capabilityRuntime{embeddedRuntime: rt, cap: runtime.RuntimeCapability{SchemaVersion: 1}}
	s.Runtime = c
	for _, check := range []struct {
		err       error
		kinds     []string
		status    string
		available bool
	}{
		{errors.New("runtime unreachable"), nil, "unavailable", false},
		{nil, []string{"codex"}, "not_installed", false},
		{nil, []string{"codex", "hermes"}, "available", true},
	} {
		c.err, c.cap.AgentKinds = check.err, check.kinds
		w := request(t, s, "GET", "/v1/projects/"+p.ID+"/hermes", "")
		var out struct {
			Status    string `json:"runtime_status"`
			Available bool   `json:"runtime_available"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Status != check.status || out.Available != check.available {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

type hermesRuntime struct {
	*embeddedRuntime
	spec runtime.WorkspaceSpec
}

type matrixCredentialRuntime struct {
	*hermesRuntime
	saves int
}

func (r *matrixCredentialRuntime) MatrixCredential(context.Context, string, shared.HermesMatrix, string) error {
	r.saves++
	return nil
}
func TestMatrixCredentialCannotFollowAChangedHomeserver(t *testing.T) {
	s, rt := setup(t)
	p := domainProject(t, s, "matrix-credential-revision")
	r := &matrixCredentialRuntime{hermesRuntime: &hermesRuntime{embeddedRuntime: rt}}
	s.Runtime = r
	s.matrixSetup = testMatrixOnboarding{}
	c := shared.HermesSetup{ProjectID: p.ID, Provider: "openai-codex", Model: "approved-model", Mode: "both", Matrix: &shared.HermesMatrix{Homeserver: "https://matrix.example.org", BotUserID: "@bot:example.org", RoomIDs: []string{"!room:example.org"}, SenderIDs: []string{"@alice:example.org"}}}
	path := "/v1/projects/" + p.ID + "/hermes"
	body, _ := json.Marshal(c)
	if w := request(t, s, "POST", path, string(body)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := request(t, s, "POST", path+"/matrix-credential", `{"access_token":"synthetic-private-token","revision":1}`)
	if w.Code != 200 || r.saves != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	c, _ = s.Store.HermesSetup(context.Background(), p.ID)
	c.Matrix.Homeserver = "https://changed.example.org"
	body, _ = json.Marshal(c)
	if w = request(t, s, "POST", path, string(body)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(t, s, "POST", path+"/matrix-credential", `{"access_token":"synthetic-private-token","revision":1}`)
	if w.Code != 409 || r.saves != 1 || strings.Contains(w.Body.String(), "synthetic-private-token") {
		t.Fatal("stale credential submission reached another origin", w.Code, w.Body.String())
	}
}

func (r *hermesRuntime) Capabilities(context.Context) (runtime.RuntimeCapability, error) {
	caps, err := r.embeddedRuntime.Capabilities(context.Background())
	caps.AgentKinds = []string{"hermes"}
	return caps, err
}
func (r *hermesRuntime) StartConfigured(ctx context.Context, id string, spec runtime.WorkspaceSpec) (shared.Observation, error) {
	r.spec = spec
	return r.Start(ctx, id)
}

func TestHermesAdmissionRequiresInstalledEngineAndForwardsSavedSettings(t *testing.T) {
	s, rt := setup(t)
	p := domainProject(t, s, "hermes-runtime")
	c := shared.HermesSetup{ProjectID: p.ID, Provider: "openai-codex", Mode: "both", Model: "approved-project-model", Matrix: &shared.HermesMatrix{Homeserver: "https://matrix.example.org", BotUserID: "@bot:example.org", RoomIDs: []string{"!room:example.org"}, SenderIDs: []string{"@alice:example.org"}}}
	b, _ := json.Marshal(c)
	if w := request(t, s, "POST", "/v1/projects/"+p.ID+"/hermes", string(b)); w.Code != 200 {
		t.Fatal(w.Code)
	}
	body := `{"project_id":"` + p.ID + `","agent_kind":"hermes","idempotency_key":"hermes-runtime-create"}`
	if w := request(t, s, "POST", "/v1/workspaces", body); w.Code != 409 {
		t.Fatal("uninstalled engine admitted", w.Code)
	}
	installed := &hermesRuntime{embeddedRuntime: rt}
	s.Runtime = installed
	w := request(t, s, "POST", "/v1/workspaces", body)
	var ws shared.Workspace
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &ws) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"hermes-start"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if installed.spec.AgentKind != "hermes" || installed.spec.HermesModel != c.Model || installed.spec.ProjectID != p.ID || installed.spec.HermesMatrix == nil || installed.spec.HermesMatrix.BotUserID != c.Matrix.BotUserID {
		t.Fatal("saved settings lost on boot", installed.spec)
	}
}

func TestHermesSettingsSaveAndRevisionFence(t *testing.T) {
	s, rt := setup(t)
	p := domainProject(t, s, "hermes")
	path := "/v1/projects/" + p.ID + "/hermes"
	get := request(t, s, "GET", path, "")
	if get.Code != 200 || !strings.Contains(get.Body.String(), `"saved":false`) {
		t.Fatal(get.Code, get.Body.String())
	}
	c := shared.HermesSetup{ProjectID: p.ID, Provider: "openai-codex", Mode: "workspace", Model: "gpt-6.1-sol"}
	b, _ := json.Marshal(c)
	w := request(t, s, "POST", path, string(b))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	stored, e := s.Store.HermesSetup(context.Background(), p.ID)
	if e != nil || stored.Revision != 1 || stored.Model != c.Model || stored.UpdatedAt == nil {
		t.Fatal("settings not persisted", stored, e)
	}
	if !strings.Contains(w.Body.String(), `"runtime_available":false`) {
		t.Fatal("saving settings claimed runtime readiness")
	}
	if w = request(t, s, "POST", path, string(b)); w.Code != 409 {
		t.Fatal("stale save overwrote settings", w.Code)
	}
	stored.Mode = "both"
	stored.Matrix = &shared.HermesMatrix{Homeserver: "https://matrix.example.org", BotUserID: "@bot:example.org", RoomIDs: []string{"!room:example.org"}, SenderIDs: []string{"@user:example.org"}}
	b, _ = json.Marshal(stored)
	if w = request(t, s, "POST", path, string(b)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	got, e := s.Store.HermesSetup(context.Background(), p.ID)
	if e != nil || got.Matrix.BotUserID != stored.Matrix.BotUserID || got.Revision != 2 {
		t.Fatal(got, e)
	}
	if rt.starts != 0 {
		t.Fatal("configuration started a workspace")
	}
}

func TestHermesSettingsProjectIsolation(t *testing.T) {
	s, _ := setup(t)
	a, b := domainProject(t, s, "hermes-a"), domainProject(t, s, "hermes-b")
	s.cfg.ProjectTokens = map[string]string{"project-a": a.ID}
	for _, method := range []string{"GET", "POST"} {
		w := domainRequest(t, s, "project-a", method, "/v1/projects/"+b.ID+"/hermes", `{}`)
		if w.Code != 404 {
			t.Fatal("foreign configuration accessible", method, w.Code)
		}
	}
	c := shared.HermesSetup{ProjectID: b.ID, Provider: "openai-codex", Mode: "workspace", Model: "gpt-6.1-sol"}
	body, _ := json.Marshal(c)
	w := domainRequest(t, s, "project-a", "POST", "/v1/projects/"+a.ID+"/hermes", string(body))
	if w.Code != 400 {
		t.Fatal("forged owner accepted", w.Code)
	}
}

func TestHermesBothCanSaveWorkspaceBeforeMatrixDetails(t *testing.T) {
	s, _ := setup(t)
	p := domainProject(t, s, "hermes-both")
	c := shared.HermesSetup{ProjectID: p.ID, Mode: "both", Provider: "openai-codex", Model: "gpt-6.1-sol"}
	b, _ := json.Marshal(c)
	w := request(t, s, "POST", "/v1/projects/"+p.ID+"/hermes", string(b))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestMatrixCredentialsAndConnectionAreOperatorOnly(t *testing.T) {
	s, _ := setup(t)
	p := domainProject(t, s, "matrix-authority")
	s.cfg.ProjectTokens = map[string]string{"project-token": p.ID}
	for _, path := range []string{"matrix-credential", "matrix/start", "matrix/stop", "discover"} {
		w := domainRequest(t, s, "project-token", "POST", "/v1/projects/"+p.ID+"/hermes/"+path, `{}`)
		if w.Code != 403 {
			t.Fatal("agent credential gained bot authority", path, w.Code)
		}
	}
}

func TestHermesValidationRejectsSecretsAndUnsafeSettings(t *testing.T) {
	s, _ := setup(t)
	p := domainProject(t, s, "validation")
	path := "/v1/projects/" + p.ID + "/hermes"
	c := shared.HermesSetup{ProjectID: p.ID, Provider: "openai-codex", Mode: "matrix", Model: "gpt-6.1-sol", Matrix: &shared.HermesMatrix{Homeserver: "https://matrix.example.org", BotUserID: "@bot:example.org", RoomIDs: []string{"!room:example.org"}, SenderIDs: []string{"@user:example.org"}}}
	for _, mutate := range []func(*shared.HermesSetup){
		func(c *shared.HermesSetup) { c.Provider = "api-key" },
		func(c *shared.HermesSetup) { c.Model = "" },
		func(c *shared.HermesSetup) { c.Model = "model\nsecret" },
		func(c *shared.HermesSetup) { c.Matrix.Homeserver = "http://matrix.example.org" },
		func(c *shared.HermesSetup) { c.Matrix.Homeserver = "https://user:secret@matrix.example.org" },
		func(c *shared.HermesSetup) { c.Matrix.Homeserver = "https://matrix.example.org?token=secret" },
		func(c *shared.HermesSetup) { c.Matrix.RoomIDs = []string{"*"} },
		func(c *shared.HermesSetup) { c.Matrix.SenderIDs = []string{} },
	} {
		b, _ := json.Marshal(c)
		var copy shared.HermesSetup
		json.Unmarshal(b, &copy)
		mutate(&copy)
		b, _ = json.Marshal(copy)
		w := request(t, s, "POST", path, string(b))
		if w.Code != 400 {
			t.Fatal("unsafe configuration accepted", w.Code, string(b))
		}
	}
	b, _ := json.Marshal(c)
	body := strings.TrimSuffix(string(b), "}") + `,"access_token":"never-store-this"}`
	if w := request(t, s, "POST", path, body); w.Code != 400 {
		t.Fatal("credential field accepted", w.Code)
	}
	c.ConnectionProfileID = "open"
	b, _ = json.Marshal(c)
	if w := request(t, s, "POST", path, string(b)); w.Code != 400 {
		t.Fatal("open profile accepted", w.Code)
	}
}

type testMatrixOnboarding struct{}

func (testMatrixOnboarding) Discover(context.Context, string) (string, error) {
	return "https://matrix.example.org", nil
}
func (testMatrixOnboarding) Prepare(context.Context, shared.HermesMatrix, string, bool) error {
	return nil
}

type matrixRestartRuntime struct {
	*hermesRuntime
	enables int
}

func (r *matrixRestartRuntime) MatrixAction(_ context.Context, _ string, action string, _ *shared.HermesMatrix) error {
	if action == "enable" {
		r.enables++
		if r.enables == 1 {
			return errors.New("MATRIX_RESTART_REQUIRED")
		}
	}
	return nil
}
func (r *matrixRestartRuntime) Agent(ctx context.Context, id, method, path string, b []byte) (int, []byte, error) {
	if method == "POST" && path == "/v1/matrix/start" {
		return 202, []byte(`{"state":"starting"}`), nil
	}
	return r.embeddedRuntime.Agent(ctx, id, method, path, b)
}
func TestMatrixConnectAppliesSavedSettingsWithoutInterruptingWork(t *testing.T) {
	for _, busy := range []bool{false, true} {
		t.Run(map[bool]string{false: "idle", true: "busy"}[busy], func(t *testing.T) {
			s, rt := setup(t)
			r := &matrixRestartRuntime{hermesRuntime: &hermesRuntime{embeddedRuntime: rt}}
			s.Runtime = r
			p := domainProject(t, s, "matrix-restart")
			c := shared.HermesSetup{ProjectID: p.ID, Provider: "openai-codex", Model: "model", Mode: "both", Matrix: &shared.HermesMatrix{Homeserver: "https://matrix.example.org", BotUserID: "@bot:example.org", RoomIDs: []string{"!room:example.org"}, SenderIDs: []string{"@alice:example.org"}}}
			b, _ := json.Marshal(c)
			if w := request(t, s, "POST", "/v1/projects/"+p.ID+"/hermes", string(b)); w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			w := request(t, s, "POST", "/v1/workspaces", `{"project_id":"`+p.ID+`","agent_kind":"hermes","idempotency_key":"matrix-restart"}`)
			var ws shared.Workspace
			if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &ws) != nil {
				t.Fatal(w.Code, w.Body.String())
			}
			w = request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"initial-start"}`)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if busy {
				rt.jobs = 1
			}
			w = request(t, s, "POST", "/v1/projects/"+p.ID+"/hermes/matrix/start", `{}`)
			if busy {
				if w.Code != 409 || rt.starts != 1 {
					t.Fatal("active workspace interrupted", w.Code, rt.starts)
				}
			} else if w.Code != 202 || rt.starts != 2 || r.enables != 2 {
				t.Fatal("idle settings not applied", w.Code, w.Body.String(), rt.starts, r.enables)
			}
		})
	}
}
