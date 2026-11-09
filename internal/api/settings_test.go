package api

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"testing"
	"time"
)

type policyRuntime struct {
	*embeddedRuntime
	enabled      bool
	applications int
}

type firecrackerSettingsRuntime struct{ *embeddedRuntime }

func (r *firecrackerSettingsRuntime) Inspect(ctx context.Context, id string) (shared.Observation, error) {
	o, err := r.embeddedRuntime.Inspect(ctx, id)
	o.RuntimeType = "firecracker"
	return o, err
}
func TestFirecrackerRefusesOpenSettingsBeforePersistingAndCanRecoverLegacyPolicy(t *testing.T) {
	s, rt := setup(t)
	s.Runtime = &firecrackerSettingsRuntime{rt}
	ws := create(t, s, "sandbox-network-settings")
	path := "/v1/workspaces/" + ws.ID
	w := request(t, s, "POST", path+"/settings", `{"idle_timeout_minutes":60,"connections":{"mode":"open","destinations":[]}}`)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	stored, _ := s.Store.Workspace(context.Background(), ws.ID)
	if stored.Connections != nil {
		t.Fatal("forbidden policy persisted")
	}
	// Reproduce metadata written by an older control plane, before the host
	// correctly refused the VM start. Keep the cause visible to the UI.
	stored.Connections = &shared.ConnectionPolicy{Mode: "open"}
	if err := s.Store.Save(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	w = request(t, s, "POST", path+"/start", `{"idempotency_key":"old-open-policy"}`)
	if w.Code != 409 || rt.starts != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(t, s, "POST", path+"/settings", `{"idle_timeout_minutes":60,"connections":{"mode":"blocked","destinations":[]}}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(t, s, "POST", path+"/start", `{"idempotency_key":"recovered-blocked-policy"}`)
	if w.Code != 200 || rt.starts != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func (r *policyRuntime) ConfigureNetwork(_ context.Context, _ string, enabled bool) error {
	r.enabled = enabled
	r.applications++
	return nil
}
func TestSettingsAndReopenControlIdleShutdown(t *testing.T) {
	s, rt := setup(t)
	policy := &policyRuntime{embeddedRuntime: rt}
	s.Runtime = policy
	if s.cfg.IdleTimeout != time.Hour {
		t.Fatal("default should allow an hour")
	}
	ws := create(t, s, "settings")
	path := "/v1/workspaces/" + ws.ID
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	save := func(minutes int, github bool) {
		t.Helper()
		payload, _ := json.Marshal(map[string]any{"idle_timeout_minutes": minutes, "github_access": github})
		body := string(payload)
		w := request(t, s, "POST", path+"/settings", body)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	save(30, true)
	if policy.applications != 0 || rt.starts != 0 {
		t.Fatal("settings woke stopped workspace")
	}
	w := request(t, s, "POST", path+"/start", `{"idempotency_key":"start"}`)
	if w.Code != 200 || !policy.enabled {
		t.Fatal("start failed to apply policy", w.Body)
	}
	now = now.Add(29 * time.Minute)
	s.Reconcile(context.Background())
	if rt.managers[ws.ID] == nil {
		t.Fatal("stopped early")
	}
	w = request(t, s, "POST", path+"/start", `{"idempotency_key":"open"}`)
	if w.Code != 200 || rt.starts != 1 {
		t.Fatal("reopening recreated workspace")
	}
	var current shared.Workspace
	json.Unmarshal(w.Body.Bytes(), &current)
	if current.IdleStopAt == nil || !current.IdleStopAt.Equal(now.Add(30*time.Minute)) {
		t.Fatal("open did not reset deadline")
	}
	now = now.Add(2 * time.Minute)
	s.Reconcile(context.Background())
	if rt.managers[ws.ID] == nil {
		t.Fatal("old deadline still used")
	}
	save(0, false)
	if policy.enabled {
		t.Fatal("revocation not applied")
	}
	now = now.Add(48 * time.Hour)
	s.Reconcile(context.Background())
	if rt.managers[ws.ID] == nil {
		t.Fatal("never setting ignored")
	}
	save(15, false)
	now = now.Add(16 * time.Minute)
	s.Reconcile(context.Background())
	if rt.managers[ws.ID] != nil {
		t.Fatal("configured idle shutdown ignored")
	}
}
func TestInvalidSettingsDoNotMutateWorkspace(t *testing.T) {
	s, _ := setup(t)
	ws := create(t, s, "invalid-settings")
	for _, body := range []string{`{}`, `{"idle_timeout_minutes":-1,"github_access":true}`, `{"idle_timeout_minutes":1,"github_access":true}`, `{"idle_timeout_minutes":1441,"github_access":true}`, `{"idle_timeout_minutes":60,"github_access":true,"domains":["evil.test"]}`} {
		w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/settings", body)
		if w.Code != 400 {
			t.Fatalf("accepted invalid settings: %d", w.Code)
		}
	}
}

func TestOpenProfileRequiresStoppedWorkspaceAndPersistsAcrossRestart(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "open-policy")
	path := "/v1/workspaces/" + ws.ID
	r := request(t, s, "POST", path+"/start", `{"idempotency_key":"start"}`)
	if r.Code != 200 {
		t.Fatal(r.Body)
	}
	r = request(t, s, "POST", path+"/settings", `{"idle_timeout_minutes":60,"connections":{"mode":"open","destinations":[]}}`)
	if r.Code != 409 {
		t.Fatal("open policy changed running networking", r.Code)
	}
	w, _ := s.Store.Workspace(context.Background(), ws.ID)
	if w.Connections != nil {
		t.Fatal("rejected change persisted")
	}
	r = request(t, s, "POST", path+"/stop", `{"idempotency_key":"stop"}`)
	if r.Code != 200 {
		t.Fatal(r.Body)
	}
	r = request(t, s, "POST", path+"/settings", `{"idle_timeout_minutes":60,"connections":{"mode":"open","destinations":[]}}`)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body)
	}
	restarted := New(s.Store, rt, Config{Token: "control-token"})
	r = request(t, restarted, "GET", path, "")
	var saved shared.Workspace
	json.Unmarshal(r.Body.Bytes(), &saved)
	if saved.Connections == nil || saved.Connections.Mode != "open" {
		t.Fatal("profile lost after control-plane restart")
	}
}
