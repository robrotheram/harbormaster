package api

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"testing"
)

type consoleRestoreRuntime struct {
	*embeddedRuntime
	enables, probes, unready, enableStatus int
}

func (rt *consoleRestoreRuntime) Agent(ctx context.Context, id, method, path string, b []byte) (int, []byte, error) {
	if method == "POST" && path == "/v1/console/enable" {
		rt.enables++
		status := rt.enableStatus
		if status == 0 {
			status = 200
		}
		return status, []byte(`{}`), nil
	}
	if method == "GET" && path == "/v1/activity" {
		rt.probes++
		b, _ := json.Marshal(shared.Activity{Known: rt.probes > rt.unready})
		return 200, b, nil
	}
	return rt.embeddedRuntime.Agent(ctx, id, method, path, b)
}

func TestStartRestoresPersistedConsoleAndWaitsForReadiness(t *testing.T) {
	for _, alreadyRunning := range []bool{false, true} {
		t.Run(map[bool]string{false: "cold-start", true: "ready-without-console"}[alreadyRunning], func(t *testing.T) {
			s, base := setup(t)
			ws := create(t, s, "console-restore")
			ws.ConsoleEnabled = true
			if err := s.Store.Save(context.Background(), ws); err != nil {
				t.Fatal(err)
			}
			if alreadyRunning {
				if _, err := base.Start(context.Background(), ws.ID); err != nil {
					t.Fatal(err)
				}
			}
			rt := &consoleRestoreRuntime{embeddedRuntime: base, unready: 1}
			s.Runtime = rt
			w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"restore"}`)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if rt.enables != 1 || rt.probes != 2 || base.starts != 1 {
				t.Fatalf("console wasn't reconciled before return: enables=%d probes=%d starts=%d", rt.enables, rt.probes, base.starts)
			}
			stored, err := s.Store.Workspace(context.Background(), ws.ID)
			if err != nil || !stored.ConsoleEnabled {
				t.Fatal("lost persisted console setting", err)
			}
		})
	}
}

func TestStartDoesNotEnableConsoleForTaskOnlyWorkspace(t *testing.T) {
	s, base := setup(t)
	ws := create(t, s, "headless")
	rt := &consoleRestoreRuntime{embeddedRuntime: base}
	s.Runtime = rt
	w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"start"}`)
	if w.Code != 200 || rt.enables != 0 {
		t.Fatal(w.Code, rt.enables)
	}
}

func TestConsoleRestorationPreservesBusyRefusal(t *testing.T) {
	s, base := setup(t)
	ws := create(t, s, "busy-console")
	ws.ConsoleEnabled = true
	if e := s.Store.Save(context.Background(), ws); e != nil {
		t.Fatal(e)
	}
	rt := &consoleRestoreRuntime{embeddedRuntime: base, enableStatus: 409}
	s.Runtime = rt
	w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"start"}`)
	if w.Code != 409 || rt.probes != 0 {
		t.Fatal(w.Code, w.Body.String(), rt.probes)
	}
}
