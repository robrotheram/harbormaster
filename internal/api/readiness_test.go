package api

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspacesvc"
)

type readinessRuntime struct {
	*embeddedRuntime
	caps            protocol.RuntimeCapability
	err, inspectErr error
	reads           int
}

func (rt *readinessRuntime) Capabilities(context.Context) (protocol.RuntimeCapability, error) {
	rt.reads++
	return rt.caps, rt.err
}
func (rt *readinessRuntime) Inspect(ctx context.Context, id string) (shared.Observation, error) {
	rt.reads++
	if rt.inspectErr != nil {
		return shared.Observation{}, rt.inspectErr
	}
	return rt.embeddedRuntime.Inspect(ctx, id)
}

func TestStartReadinessIsReadOnlyAndProjectScoped(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "readiness")
	probe := &readinessRuntime{embeddedRuntime: rt, caps: protocol.RuntimeCapability{SchemaVersion: 1, NetworkModes: []string{"blocked"}, Resources: protocol.ResourceLimits{CPUCores: ws.CPULimit, MemoryBytes: ws.MemoryLimitBytes}}}
	s.Runtime = probe
	s.cfg.ProjectTokens = map[string]string{"own-project": "default", "foreign-project": "foreign"}
	path := "/v1/workspaces/" + ws.ID + "/start-readiness"
	if got := domainRequest(t, s, "foreign-project", "GET", path, ""); got.Code != 404 {
		t.Fatalf("cross-project readiness: %d", got.Code)
	}
	if probe.reads != 0 {
		t.Fatal("unauthorized request contacted runtime")
	}
	before, _ := s.Store.Workspace(context.Background(), ws.ID)
	for i := 0; i < 2; i++ {
		response := domainRequest(t, s, "own-project", "GET", path, "")
		var out workspacesvc.StartReadiness
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &out) != nil || !out.CanStart || out.ProjectID != ws.ProjectID || out.WorkspaceID != ws.ID || out.CheckedAt.IsZero() {
			t.Fatalf("readiness: %d %s", response.Code, response.Body)
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("readiness must not be cached")
		}
	}
	after, _ := s.Store.Workspace(context.Background(), ws.ID)
	if !reflect.DeepEqual(before, after) || rt.starts != 0 {
		t.Fatal("readiness changed metadata or started compute")
	}
	// Capacity can change after a successful check. The real start must still reject it.
	s.cfg.CPUQuota = ws.CPULimit / 2
	if got := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"after-readiness"}`); got.Code != 429 {
		t.Fatalf("stale readiness bypassed admission: %d %s", got.Code, got.Body)
	}
	if rt.starts != 0 {
		t.Fatal("quota refusal started compute")
	}
}

func TestStartReadinessBlockers(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		configure  func(*Server, *readinessRuntime, *shared.Workspace)
	}{
		{"old controller", "RUNTIME_CAPABILITIES_INCOMPATIBLE", func(s *Server, rt *readinessRuntime, w *shared.Workspace) {
			rt.err = protocol.ErrCapabilitiesIncompatible
		}},
		{"legacy schema", "RUNTIME_CAPABILITIES_INCOMPATIBLE", func(s *Server, rt *readinessRuntime, w *shared.Workspace) { rt.caps.SchemaVersion = 0 }},
		{"offline", "RUNTIME_UNAVAILABLE", func(s *Server, rt *readinessRuntime, w *shared.Workspace) {
			rt.err = errors.New("private-host-token-must-not-leak")
			rt.inspectErr = rt.err
		}},
		{"allocation", "RESOURCE_LIMIT_UNAVAILABLE", func(s *Server, rt *readinessRuntime, w *shared.Workspace) { rt.caps.Resources.MemoryBytes *= 2 }},
		{"network", "NETWORK_POLICY_UNAVAILABLE", func(s *Server, rt *readinessRuntime, w *shared.Workspace) {
			rt.caps.NetworkModes = []string{"allowlist"}
		}},
		{"capacity", "QUOTA_EXCEEDED", func(s *Server, rt *readinessRuntime, w *shared.Workspace) { s.cfg.MemoryQuota = w.MemoryLimitBytes / 2 }},
		{"profile", "INVALID_CONNECTION_PROFILE", func(s *Server, rt *readinessRuntime, w *shared.Workspace) { w.ConnectionProfileID = "missing-profile" }},
		{"hermes", "HERMES_SETUP_REQUIRED", func(s *Server, rt *readinessRuntime, w *shared.Workspace) { w.AgentKind = "hermes" }},
		{"branch", "BRANCH_SEED_UNAVAILABLE", func(s *Server, rt *readinessRuntime, w *shared.Workspace) { w.ParentID = "ws_parent" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, rt := setup(t)
			ws := create(t, s, "readiness")
			probe := &readinessRuntime{embeddedRuntime: rt, caps: protocol.RuntimeCapability{SchemaVersion: 1, NetworkModes: []string{"blocked"}, Resources: protocol.ResourceLimits{CPUCores: ws.CPULimit, MemoryBytes: ws.MemoryLimitBytes}}}
			s.Runtime = probe
			tc.configure(s, probe, &ws)
			before, _ := s.Store.Workspace(context.Background(), ws.ID)
			out := s.lifecycle().StartReadiness(context.Background(), ws)
			payload, _ := json.Marshal(out)
			if out.CanStart {
				t.Fatalf("unsafe readiness: %s", payload)
			}
			found := false
			for _, check := range out.Checks {
				if check.Code == tc.code && check.Status != "passed" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %s: %s", tc.code, payload)
			}
			after, _ := s.Store.Workspace(context.Background(), ws.ID)
			if !reflect.DeepEqual(before, after) || rt.starts != 0 {
				t.Fatal("blocked readiness changed state")
			}
			for _, check := range out.Checks {
				if check.Message == "private-host-token-must-not-leak" {
					t.Fatal("raw runtime error exposed")
				}
			}
		})
	}
}

func TestIncompatibleRuntimeHasActionableErrorCode(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "readiness")
	s.Runtime = &readinessRuntime{embeddedRuntime: rt, err: protocol.ErrCapabilitiesIncompatible}
	response := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"start-old-runtime"}`)
	var out shared.APIError
	if response.Code != 503 || json.Unmarshal(response.Body.Bytes(), &out) != nil || out.Code != "RUNTIME_CAPABILITIES_INCOMPATIBLE" {
		t.Fatalf("startup error: %d %s", response.Code, response.Body)
	}
	if rt.starts != 0 {
		t.Fatal("incompatible runtime admitted")
	}
}

func TestStartRejectsUnsupportedCapabilitySchemasBeforeAllocating(t *testing.T) {
	for _, version := range []int{0, 2} {
		s, rt := setup(t)
		ws := create(t, s, "unsupported-schema")
		caps, _ := rt.Capabilities(t.Context())
		caps.SchemaVersion = version
		s.Runtime = &readinessRuntime{embeddedRuntime: rt, caps: caps}
		got := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"unsupported-schema"}`)
		if got.Code != 503 || !strings.Contains(got.Body.String(), "RUNTIME_CAPABILITIES_INCOMPATIBLE") {
			t.Fatalf("schema %d: %d %s", version, got.Code, got.Body)
		}
		saved, err := s.Store.Workspace(t.Context(), ws.ID)
		if err != nil || saved.State != "STOPPED" || rt.starts != 0 {
			t.Fatalf("schema %d reserved or allocated compute: %s %d %v", version, saved.State, rt.starts, err)
		}
	}
}
