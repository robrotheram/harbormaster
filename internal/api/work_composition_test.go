package api

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type lostWorkAckRuntime struct {
	*embeddedRuntime
	lost       bool
	deliveries int
}

func (rt *lostWorkAckRuntime) Agent(ctx context.Context, id, method, path string, body []byte) (int, []byte, error) {
	status, result, err := rt.embeddedRuntime.Agent(ctx, id, method, path, body)
	if path == "/v1/tasks/from-file" {
		rt.deliveries++
		if !rt.lost && err == nil && (status == 200 || status == 202) {
			rt.lost = true
			return 0, nil, fmt.Errorf("lost durable acceptance acknowledgement")
		}
	}
	return status, result, err
}
func (rt *lostWorkAckRuntime) Inspect(ctx context.Context, id string) (shared.Observation, error) {
	observation, err := rt.embeddedRuntime.Inspect(ctx, id)
	// Lost acknowledgement can leave a real task executing. Recovery must resolve
	// its existing receipt even when the next observation reports busy.
	if rt.lost && observation.Ready {
		observation.Activity.ActiveRuns = 1
	}
	return observation, err
}
func TestWorkLostAcknowledgementResolvesRealBusyWorkspaceReceipt(t *testing.T) {
	s, base := setup(t)
	rt := &lostWorkAckRuntime{embeddedRuntime: base}
	s.Runtime = rt
	project := domainProject(t, s, "work-composition")
	ws := domainWorkspace(t, s, "control-token", project.ID, "composition")
	if got := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"start"}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	specification := []byte("Describe the workspace.")
	if err := os.WriteFile(filepath.Join(base.base, ws.ID, "files", "work.md"), specification, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(specification)
	body, _ := json.Marshal(map[string]any{"project_id": project.ID, "workspace_id": ws.ID, "specification_path": "work.md", "specification_digest": hex.EncodeToString(digest[:]), "kind": "software", "idempotency_key": "create"})
	created := request(t, s, "POST", "/v1/work-items", string(body))
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	var item shared.WorkItem
	json.Unmarshal(created.Body.Bytes(), &item)
	run := request(t, s, "POST", "/v1/work-items/"+item.ID+"/run", `{"project_id":"`+project.ID+`","idempotency_key":"attempt"}`)
	if run.Code != 202 {
		t.Fatal(run.Code, run.Body.String())
	}
	s.tickOrchestration(context.Background())
	s.tickOrchestration(context.Background())
	if rt.deliveries != 2 {
		t.Fatalf("busy recovery did not retry durable receipt: %d deliveries", rt.deliveries)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		s.tickOrchestration(context.Background())
		response := request(t, s, "GET", "/v1/work-items/"+item.ID+"?project_id="+project.ID, "")
		json.Unmarshal(response.Body.Bytes(), &item)
		if item.State == "awaiting_review" {
			stale := request(t, s, "POST", "/v1/work-items/"+item.ID+"/review", `{"project_id":"`+project.ID+`","accepted":true,"expected_attempt_id":"attempt_stale"}`)
			if stale.Code != 409 {
				t.Fatal("stale evidence accepted", stale.Code)
			}
			accepted := request(t, s, "POST", "/v1/work-items/"+item.ID+"/review", `{"project_id":"`+project.ID+`","accepted":true,"expected_attempt_id":"`+item.LatestAttemptID+`"}`)
			if accepted.Code != 200 {
				t.Fatal(accepted.Code, accepted.Body.String())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("work did not reach review: %+v", item)
}
