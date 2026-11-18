package api

import (
	runtime "agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type globalProfileRuntime struct {
	*embeddedRuntime
	mode   string
	policy shared.ConnectionPolicy
}

func (r *globalProfileRuntime) Inspect(ctx context.Context, id string) (shared.Observation, error) {
	o, e := r.embeddedRuntime.Inspect(ctx, id)
	o.ConnectionMode = r.mode
	return o, e
}
func (r *globalProfileRuntime) StartConfigured(ctx context.Context, id string, spec runtime.WorkspaceSpec) (shared.Observation, error) {
	r.mode = "restricted"
	if spec.ConnectionMode == "open" {
		r.mode = "open"
	}
	return r.embeddedRuntime.StartConfigured(ctx, id, spec)
}
func (r *globalProfileRuntime) ConfigureConnections(_ context.Context, _ string, p shared.ConnectionPolicy) error {
	r.policy = p
	return nil
}
func TestGlobalProfileVersioningAndWorkspaceAdoption(t *testing.T) {
	s, rt := setup(t)
	r := &globalProfileRuntime{embeddedRuntime: rt}
	s.Runtime = r
	result := request(t, s, "GET", "/v1/connection-profiles", "")
	if result.Code != 200 {
		t.Fatal(result.Code)
	}
	var defaults struct{ Items []shared.ConnectionProfile }
	json.Unmarshal(result.Body.Bytes(), &defaults)
	if len(defaults.Items) != 4 {
		t.Fatal("missing seeded profiles")
	}
	result = request(t, s, "POST", "/v1/connection-profiles", `{"name":"Team development","description":"Shared tooling","policy":{"mode":"allowlist","destinations":["github.com:443"]},"revision":0}`)
	if result.Code != 201 {
		t.Fatal(result.Code, result.Body)
	}
	var p shared.ConnectionProfile
	json.Unmarshal(result.Body.Bytes(), &p)
	a, b := create(t, s, "linked-a"), create(t, s, "linked-b")
	for _, w := range []shared.Workspace{a, b} {
		result = request(t, s, "POST", "/v1/workspaces/"+w.ID+"/settings", `{"idle_timeout_minutes":60,"connection_profile_id":"`+p.ID+`"}`)
		if result.Code != 200 {
			t.Fatal(result.Code, result.Body)
		}
	}
	result = request(t, s, "POST", "/v1/workspaces/"+a.ID+"/start", `{"idempotency_key":"start"}`)
	if result.Code != 200 {
		t.Fatal(result.Code, result.Body)
	}
	result = request(t, s, "POST", "/v1/connection-profiles/"+p.ID, `{"name":"Team development","description":"Updated tooling","policy":{"mode":"allowlist","destinations":["github.com:443","registry.npmjs.org:443"]},"revision":1}`)
	if result.Code != 200 {
		t.Fatal(result.Code, result.Body)
	}
	old, _ := s.Store.Workspace(context.Background(), a.ID)
	if old.ConnectionProfileRevision != 1 || len(old.Connections.Destinations) != 1 || len(r.policy.Destinations) != 1 {
		t.Fatal("global edit changed running policy silently")
	}
	// Internal accesses to running workspaces retain the applied policy snapshot.
	current, _ := s.Store.Workspace(context.Background(), a.ID)
	if err := s.start(context.Background(), &current); err != nil {
		t.Fatal(err)
	}
	if current.ConnectionProfileRevision != 1 || len(r.policy.Destinations) != 1 {
		t.Fatal("background access silently adopted global changes")
	}
	result = request(t, s, "POST", "/v1/connection-profiles/"+p.ID, `{"name":"Stale edit","policy":{"mode":"open","destinations":[]},"revision":1}`)
	if result.Code != 409 {
		t.Fatal("stale edit overwrote profile")
	}
	result = request(t, s, "POST", "/v1/workspaces/"+a.ID+"/start", `{"idempotency_key":"reopen"}`)
	if result.Code != 200 {
		t.Fatal(result.Code, result.Body)
	}
	if len(r.policy.Destinations) != 2 || rt.starts != 1 {
		t.Fatal("ready reopen did not adopt latest profile without recreation")
	}
	result = request(t, s, "POST", "/v1/connection-profiles/"+p.ID, `{"name":"Team development","description":"Open","policy":{"mode":"open","destinations":[]},"revision":2}`)
	if result.Code != 200 {
		t.Fatal(result.Body)
	}
	result = request(t, s, "POST", "/v1/workspaces/"+a.ID+"/start", `{"idempotency_key":"needs-stop"}`)
	if result.Code != 409 {
		t.Fatal("changed direct networking live", result.Code)
	}
	old, _ = s.Store.Workspace(context.Background(), a.ID)
	if old.ConnectionProfileRevision != 2 || old.Connections.Mode != "allowlist" {
		t.Fatal("rejected update marked applied")
	}
	result = request(t, s, "POST", "/v1/workspaces/"+a.ID+"/stop", `{"idempotency_key":"stop"}`)
	if result.Code != 200 {
		t.Fatal(result.Body)
	}
	result = request(t, s, "POST", "/v1/workspaces/"+a.ID+"/start", `{"idempotency_key":"restart"}`)
	if result.Code != 200 || r.policy.Mode != "open" {
		t.Fatal("stopped restart failed to apply global open", result.Code, result.Body)
	}
	result = request(t, s, "POST", "/v1/workspaces/"+b.ID+"/settings", `{"idle_timeout_minutes":60,"connections":{"mode":"blocked","destinations":[]}}`)
	if result.Code != 200 {
		t.Fatal(result.Body)
	}
	detached, _ := s.Store.Workspace(context.Background(), b.ID)
	if detached.ConnectionProfileID != "" {
		t.Fatal("workspace could not detach")
	}
	restarted := New(s.Store, r, Config{Token: "control-token"})
	result = request(t, restarted, "GET", "/v1/connection-profiles", "")
	var list struct{ Items []shared.ConnectionProfile }
	json.Unmarshal(result.Body.Bytes(), &list)
	found := false
	for _, current := range list.Items {
		if current.ID == p.ID && current.Revision == 3 {
			found = true
		}
	}
	if !found {
		t.Fatal("global edit did not persist")
	}
}
func TestProjectAgentsCannotEditGlobalProfiles(t *testing.T) {
	s, _ := setup(t)
	project := domainProject(t, s, "profile-project")
	s.cfg.ProjectTokens = map[string]string{"project-profile-token": project.ID}
	for _, path := range []string{"/v1/connection-profiles", "/v1/connection-profiles/github"} {
		w := domainRequest(t, s, "project-profile-token", "POST", path, `{"name":"Escalation","policy":{"mode":"open","destinations":[]},"revision":1}`)
		if w.Code != 403 {
			t.Fatal("project credential edited global rules", w.Code)
		}
	}
}
func TestGlobalProfileCreationAndValidation(t *testing.T) {
	s, _ := setup(t)
	result := request(t, s, "POST", "/v1/workspaces", `{"project_id":"default","name":"Global on create","agent_kind":"codex","idempotency_key":"global-create","connection_profile_id":"github"}`)
	if result.Code != 201 {
		t.Fatal(result.Code, result.Body)
	}
	var w shared.Workspace
	json.Unmarshal(result.Body.Bytes(), &w)
	if w.ConnectionProfileID != "github" || w.ConnectionProfileRevision != 1 || !w.EffectiveConnections().Allows("github.com:443") {
		t.Fatal("global profile not assigned at creation")
	}
	for _, body := range []string{`{"name":"","revision":0,"policy":{"mode":"blocked","destinations":[]}}`, `{"name":"Bad","revision":0,"policy":{"mode":"allowlist","destinations":["*.example.com:443"]}}`} {
		r := request(t, s, "POST", "/v1/connection-profiles", body)
		if r.Code != 400 {
			t.Fatal("invalid profile accepted")
		}
	}
}

func TestDeleteConnectionProfile(t *testing.T) {
	s, _ := setup(t)
	project := domainProject(t, s, "delete-profile")
	s.cfg.ProjectTokens = map[string]string{"project-profile-token": project.ID}
	path := "/v1/connection-profiles/github/delete"
	if got := domainRequest(t, s, "project-profile-token", "POST", path, `{"revision":1}`); got.Code != 403 {
		t.Fatalf("project credential deleted global profile: %d", got.Code)
	}
	for _, body := range []string{`{}`, `{"revision":0}`, `{"revision":-1}`} {
		if got := request(t, s, "POST", path, body); got.Code != 400 {
			t.Fatalf("invalid revision: %d", got.Code)
		}
	}
	if got := request(t, s, "POST", path, `{"revision":2}`); got.Code != 409 {
		t.Fatalf("stale deletion: %d", got.Code)
	}
	w := create(t, s, "delete-linked")
	if got := request(t, s, "POST", "/v1/workspaces/"+w.ID+"/settings", `{"idle_timeout_minutes":60,"connection_profile_id":"github"}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	if got := request(t, s, "POST", path, `{"revision":1}`); got.Code != 409 || !strings.Contains(got.Body.String(), "CONNECTION_PROFILE_IN_USE") {
		t.Fatal(got.Code, got.Body)
	}
	if got := request(t, s, "POST", "/v1/workspaces/"+w.ID+"/settings", `{"idle_timeout_minutes":60,"connection_profile_id":"blocked"}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	setup := shared.HermesSetup{ProjectID: project.ID, ConnectionProfileID: "github", Revision: 1}
	if err := s.Store.SaveHermesSetup(context.Background(), setup, 0); err != nil {
		t.Fatal(err)
	}
	if got := request(t, s, "POST", path, `{"revision":1}`); got.Code != 409 || !strings.Contains(got.Body.String(), "CONNECTION_PROFILE_IN_USE") {
		t.Fatal(got.Code, got.Body)
	}
	setup.ConnectionProfileID = "blocked"
	setup.Revision++
	if err := s.Store.SaveHermesSetup(context.Background(), setup, 1); err != nil {
		t.Fatal(err)
	}
	if got := request(t, s, "POST", path, `{"revision":1}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	profiles, err := s.Store.ConnectionProfiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range profiles {
		if p.ID == "github" {
			t.Fatal("deleted profile still listed")
		}
	}
	if got := request(t, s, "POST", path, `{"revision":1}`); got.Code != 409 {
		t.Fatalf("repeated deletion: %d", got.Code)
	}
}
