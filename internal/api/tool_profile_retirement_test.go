package api

import (
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRetiredToolProfileEndpointsAndCreateFieldAreRejected(t *testing.T) {
	s, _ := setup(t)
	for _, path := range []string{"/v1/profiles", "/v1/tool-profiles", "/v1/tool-profiles/old-custom"} {
		for _, method := range []string{"GET", "POST"} {
			if w := request(t, s, method, path, `{}`); w.Code != 404 {
				t.Fatalf("retired route %s %s: %d", method, path, w.Code)
			}
		}
	}
	w := request(t, s, "POST", "/v1/workspaces", `{"project_id":"default","profile_id":"general","idempotency_key":"retired-selector"}`)
	if w.Code != 400 {
		t.Fatalf("retired selector accepted: %d %s", w.Code, w.Body.String())
	}
}

type fixedToolsRuntime struct {
	*consoleRestoreRuntime
	spec        protocol.WorkspaceSpec
	attachments int
}

func (rt *fixedToolsRuntime) StartConfigured(ctx context.Context, id string, spec protocol.WorkspaceSpec) (shared.Observation, error) {
	rt.spec = spec
	return rt.consoleRestoreRuntime.StartConfigured(ctx, id, spec)
}
func (rt *fixedToolsRuntime) Desktop(w http.ResponseWriter, r *http.Request, id, path string) {
	rt.attachments++
	w.WriteHeader(200)
}

func TestLegacyCustomProfileStartsWithoutCatalogAndKeepsPrivateIdentity(t *testing.T) {
	s, base := setup(t)
	ws := create(t, s, "legacy-custom-profile")
	// Reproduce persisted pre-retirement metadata, including a custom profile whose
	// catalog is no longer present. Keep the exact existing home and workspace ID.
	ws.ProfileID = "retired-custom-profile"
	ws.AgentKind = ""
	raw, _ := json.Marshal(ws)
	if _, err := s.Store.DB.Exec(`UPDATE workspaces SET metadata=$1 WHERE id=$2`, string(raw), ws.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB.Exec(`DROP TABLE tool_profiles`); err != nil {
		t.Fatal(err)
	}
	rt := &fixedToolsRuntime{consoleRestoreRuntime: &consoleRestoreRuntime{embeddedRuntime: base}}
	s.Runtime = rt
	w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"legacy-start"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if rt.spec.AgentKind != "t3" || rt.spec.HomeID != ws.HomeID || rt.spec.ProjectID != ws.ProjectID {
		t.Fatalf("legacy identity changed: %+v", rt.spec)
	}
	body, _ := json.Marshal(rt.spec)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	if fields["profile_id"] != nil || fields["mcps"] != nil {
		t.Fatalf("runtime launch still carries tool selection: %s", body)
	}
	stored, err := s.Store.Workspace(context.Background(), ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ProfileID != ws.ProfileID || stored.HomeID != ws.HomeID {
		t.Fatal("legacy metadata rewritten")
	}
	// Branch creation no longer compares retired profile IDs.
	branch := request(t, s, "POST", "/v1/workspaces", `{"project_id":"default","parent_id":"`+ws.ID+`","branch":"new-branch","idempotency_key":"new-profile-free-branch"}`)
	if branch.Code != 201 {
		t.Fatal(branch.Code, branch.Body.String())
	}
	attach := request(t, s, "GET", "/v1/workspaces/"+ws.ID+"/desktop/status", "")
	if attach.Code != 200 || rt.attachments != 1 {
		t.Fatal("legacy native attachment was lost", attach.Code)
	}
}

func TestConsoleEligibilityUsesAgentKindAndPersistedUpgrade(t *testing.T) {
	s, base := setup(t)
	rt := &fixedToolsRuntime{consoleRestoreRuntime: &consoleRestoreRuntime{embeddedRuntime: base}}
	s.Runtime = rt
	w := request(t, s, "POST", "/v1/workspaces", `{"project_id":"default","agent_kind":"codex","idempotency_key":"headless-modern"}`)
	var ws shared.Workspace
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &ws) != nil {
		t.Fatal(w.Body.String())
	}
	// An old general-profile marker must not implicitly upgrade explicit Codex.
	ws.ProfileID = "general"
	raw, _ := json.Marshal(ws)
	if _, err := s.Store.DB.Exec(`UPDATE workspaces SET metadata=$1 WHERE id=$2`, string(raw), ws.ID); err != nil {
		t.Fatal(err)
	}
	attach := request(t, s, "GET", "/v1/workspaces/"+ws.ID+"/desktop/status", "")
	if attach.Code != 409 || rt.attachments != 0 {
		t.Fatal("profile marker enabled a headless workspace")
	}
	ws.ConsoleEnabled = true
	if err := s.Store.Save(context.Background(), ws); err != nil {
		t.Fatal(err)
	}
	attach = httptest.NewRecorder()
	s.Handler().ServeHTTP(attach, authorizedRequest("GET", "/v1/workspaces/"+ws.ID+"/desktop/status"))
	if attach.Code != 200 || rt.attachments != 1 {
		t.Fatal("persisted console upgrade was ignored", attach.Code)
	}
}
func authorizedRequest(method, path string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Authorization", "Bearer control-token")
	return r
}

func TestProfileFreeCreateKeepsLegacyReplayAndModernFingerprint(t *testing.T) {
	s, _ := setup(t)
	old := `{"id":"ws_legacy_replay","project_id":"default","state":"STOPPED"}`
	if _, err := s.Store.DB.Exec(`INSERT INTO workspaces(id,create_key,metadata) VALUES('ws_legacy_replay','legacy-create',$1)`, old); err != nil {
		t.Fatal(err)
	}
	w := request(t, s, "POST", "/v1/workspaces", `{"project_id":"default","idempotency_key":"legacy-create"}`)
	var legacy shared.Workspace
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &legacy) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if legacy.ID != "ws_legacy_replay" || legacy.HomeID != "" || legacy.AgentKind != "codex" {
		t.Fatal("legacy replay changed identity", legacy)
	}
	modern := create(t, s, "modern-fingerprint")
	w = request(t, s, "POST", "/v1/workspaces", `{"project_id":"default","idempotency_key":"modern-fingerprint","name":"different"}`)
	if w.Code != 409 {
		t.Fatal("modern profile-free replay lost fingerprint", w.Code)
	}
	stored, err := s.Store.Workspace(context.Background(), modern.ID)
	if err != nil || stored.Name != "" {
		t.Fatal("conflicting replay changed metadata", err)
	}
}
