package api

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type lostWorkAckRuntime struct {
	*embeddedRuntime
	lost       bool
	deliveries int
	kind       string
}

func (rt *lostWorkAckRuntime) Agent(ctx context.Context, id, method, path string, body []byte) (int, []byte, error) {
	status, result, err := rt.embeddedRuntime.Agent(ctx, id, method, path, body)
	if path == "/v1/tasks/from-file" {
		rt.deliveries++
		var in struct {
			Kind string `json:"work_kind"`
		}
		if err := json.Unmarshal(body, &in); err != nil {
			return 0, nil, err
		}
		rt.kind = in.Kind
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
	body, _ := json.Marshal(map[string]any{"project_id": project.ID, "workspace_id": ws.ID, "specification_path": "work.md", "specification_digest": hex.EncodeToString(digest[:]), "kind": shared.WorkKindDesktop, "idempotency_key": "create"})
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
	if rt.kind != shared.WorkKindDesktop {
		t.Fatalf("desktop profile lost during dispatch: %q", rt.kind)
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
			var reviewed shared.WorkItem
			if err := json.Unmarshal(accepted.Body.Bytes(), &reviewed); err != nil || reviewed.State != "completed" {
				t.Fatalf("accepted work must return the completed contract: %s", accepted.Body.String())
			}
			response = request(t, s, "GET", "/v1/work-items/"+item.ID+"?project_id="+project.ID, "")
			if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &reviewed) != nil || reviewed.State != "completed" {
				t.Fatalf("acceptance did not persist: %d %s", response.Code, response.Body.String())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("work did not reach review: %+v", item)
}

func TestInlineWorkPrivatePublicationRetry(t *testing.T) {
	s, rt := setup(t)
	project := domainProject(t, s, "inline-work")
	body := `{"project_id":"` + project.ID + `","instruction":"private novel instruction","kind":"software","idempotency_key":"inline"}`
	var item shared.WorkItem
	for _, expected := range []int{201, 200} {
		response := request(t, s, "POST", "/v1/work-items", body)
		if response.Code != expected {
			t.Fatal(response.Code, response.Body.String())
		}
		if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
	}
	if item.State != "draft" {
		t.Fatal(item.State)
	}
	content, err := os.ReadFile(filepath.Join(rt.base, item.WorkspaceID, "files", item.SpecificationPath))
	if err != nil || string(content) != "private novel instruction" {
		t.Fatal(string(content), err)
	}
	workspaces, err := s.Store.Workspaces(context.Background())
	if err != nil || len(workspaces) != 1 {
		t.Fatal(workspaces, err)
	}
	ws := workspaces[0]
	if ws.AgentKind != "codex" || ws.TemplateID != "development-v1" || ws.CPULimit != 1 || ws.MemoryLimitBytes != s.cfg.WorkspaceMemory {
		t.Fatal(ws)
	}
	var metadata string
	if err := s.Store.DB.QueryRow(`SELECT metadata FROM work_items WHERE id=$1`, item.ID).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, "private novel instruction") {
		t.Fatal("instruction leaked into SQL")
	}
	var attempts int
	if err := s.Store.DB.QueryRow(`SELECT count(*) FROM work_attempts`).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatal(attempts, err)
	}
	changed := strings.Replace(body, "private novel instruction", "changed instruction", 1)
	if response := request(t, s, "POST", "/v1/work-items", changed); response.Code != 409 {
		t.Fatal(response.Code, response.Body.String())
	}
}

type lostPublicationRuntime struct {
	*embeddedRuntime
	failBefore, loseAfter, busyBefore bool
	modelCalls, fileReads             int
}

func (rt *lostPublicationRuntime) Agent(ctx context.Context, id, method, path string, body []byte) (int, []byte, error) {
	if strings.Contains(path, "/tasks") {
		rt.modelCalls++
	}
	if path == "/v1/files/read" {
		rt.fileReads++
	}
	if path == "/v1/files/write-new" && rt.busyBefore {
		rt.busyBefore = false
		return http.StatusConflict, []byte(`{"code":"WORKSPACE_BUSY"}`), nil
	}
	if path == "/v1/files/write-new" && rt.failBefore {
		rt.failBefore = false
		return 0, nil, fmt.Errorf("write unavailable")
	}
	status, data, err := rt.embeddedRuntime.Agent(ctx, id, method, path, body)
	if path == "/v1/files/write-new" && rt.loseAfter && status == 201 {
		rt.loseAfter = false
		return 0, nil, fmt.Errorf("lost publication acknowledgement")
	}
	return status, data, err
}
func TestInlineWorkReconcilesPartialPublication(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			s, base := setup(t)
			rt := &lostPublicationRuntime{embeddedRuntime: base, failBefore: !lost, loseAfter: lost}
			s.Runtime = rt
			project := domainProject(t, s, "partial-inline")
			body := `{"project_id":"` + project.ID + `","instruction":"immutable input","kind":"research","idempotency_key":"partial"}`
			if got := request(t, s, "POST", "/v1/work-items", body); got.Code != 503 {
				t.Fatal(got.Code, got.Body.String())
			}
			workers, err := s.Store.Workspaces(context.Background())
			if err != nil || len(workers) != 1 {
				t.Fatal(workers, err)
			}
			if got := request(t, s, "POST", "/v1/work-items", strings.Replace(body, "immutable input", "changed", 1)); got.Code != 409 {
				t.Fatal(got.Code, got.Body.String())
			}
			got := request(t, s, "POST", "/v1/work-items", body)
			if got.Code != 201 {
				t.Fatal(got.Code, got.Body.String())
			}
			var item shared.WorkItem
			json.Unmarshal(got.Body.Bytes(), &item)
			if item.WorkspaceID != workers[0].ID || rt.modelCalls != 0 {
				t.Fatal(item, rt.modelCalls)
			}
			workers, err = s.Store.Workspaces(context.Background())
			if err != nil || len(workers) != 1 {
				t.Fatal(workers, err)
			}
		})
	}
}

func TestInlineWorkRetriesAfterWorkerBusy(t *testing.T) {
	s, base := setup(t)
	rt := &lostPublicationRuntime{embeddedRuntime: base, busyBefore: true}
	s.Runtime = rt
	project := domainProject(t, s, "busy-inline")
	worker := domainWorkspace(t, s, "control-token", project.ID, "busy-worker")
	body := `{"project_id":"` + project.ID + `","workspace_id":"` + worker.ID + `","instruction":"Analyze the data","kind":"data-science","idempotency_key":"busy"}`
	got := request(t, s, "POST", "/v1/work-items", body)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), `"code":"WORKSPACE_BUSY"`) || rt.fileReads != 0 {
		t.Fatalf("busy worker treated as a possible write: %d %s, reads=%d", got.Code, got.Body, rt.fileReads)
	}
	got = request(t, s, "POST", "/v1/work-items", body)
	if got.Code != http.StatusCreated || rt.modelCalls != 0 {
		t.Fatalf("retry did not save a single draft: %d %s", got.Code, got.Body)
	}
}

func TestEmptyThreadRunsOnlyAfterFirstMessage(t *testing.T) {
	s, _ := setup(t)
	project := domainProject(t, s, "empty-conversation")
	body := `{"project_id":"` + project.ID + `","empty_thread":true,"kind":"data-science","provider":"codex","model":"worker-model","idempotency_key":"empty-thread"}`
	created := request(t, s, "POST", "/v1/work-items", body)
	var item shared.WorkItem
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &item) != nil || item.State != "draft" || item.SpecificationPath != "" || item.LatestAttemptID != "" {
		t.Fatalf("expected an empty thread: %d %s", created.Code, created.Body)
	}
	path := "/v1/work-items/" + item.ID
	run := request(t, s, "POST", path+"/run", `{"project_id":"`+project.ID+`","idempotency_key":"premature"}`)
	if run.Code != 409 {
		t.Fatalf("empty thread ran before a message: %d %s", run.Code, run.Body)
	}
	message := `{"project_id":"` + project.ID + `","instruction":"Analyze the measurements.","idempotency_key":"first-message"}`
	var first shared.WorkAttempt
	for _, status := range []int{202, 200} {
		response := request(t, s, "POST", path+"/continue", message)
		var attempt shared.WorkAttempt
		if response.Code != status || json.Unmarshal(response.Body.Bytes(), &attempt) != nil || attempt.ParentAttemptID != "" || attempt.SpecificationPath == "" || attempt.Provider != "codex" || attempt.Model != "worker-model" {
			t.Fatalf("first message: %d %s", response.Code, response.Body)
		}
		if status == 200 && attempt.ID != first.ID {
			t.Fatal("retry created another attempt")
		}
		first = attempt
	}
	retry := request(t, s, "POST", "/v1/work-items", body)
	if retry.Code != 200 || json.Unmarshal(retry.Body.Bytes(), &item) != nil || item.ID != first.WorkItemID || item.LatestAttemptID != first.ID {
		t.Fatalf("thread creation retry lost the first message: %d %s", retry.Code, retry.Body)
	}
}

func TestInlineWorkRejectsForeignAndOversizedBeforeProvisioning(t *testing.T) {
	s, _ := setup(t)
	a, b := domainProject(t, s, "inline-a"), domainProject(t, s, "inline-b")
	ws := domainWorkspace(t, s, "control-token", b.ID, "foreign")
	for _, tc := range []struct {
		worker, instruction string
		status              int
	}{{ws.ID, "request", 404}, {"", strings.Repeat("x", 65537), 400}} {
		body, _ := json.Marshal(map[string]any{"project_id": a.ID, "workspace_id": tc.worker, "instruction": tc.instruction, "kind": "software", "idempotency_key": "denied"})
		got := request(t, s, "POST", "/v1/work-items", string(body))
		if got.Code != tc.status {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	all, err := s.Store.Workspaces(context.Background())
	if err != nil || len(all) != 1 {
		t.Fatal(all, err)
	}
}

func TestInlineWorkQuotaIsObservableAndRetryable(t *testing.T) {
	s, _ := setup(t)
	project := domainProject(t, s, "inline-quota")
	for i := 0; i < 100; i++ {
		ws := shared.Workspace{ID: fmt.Sprintf("ws_quota_%d", i), HomeID: fmt.Sprintf("home_quota_%d", i), AgentKind: "codex", ProjectID: project.ID, State: "STOPPED", CreatedAt: s.now(), UpdatedAt: s.now()}
		if err := s.Store.Create(context.Background(), ws, fmt.Sprintf("quota-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	body := `{"project_id":"` + project.ID + `","instruction":"quota retry","kind":"software","idempotency_key":"quota"}`
	got := request(t, s, "POST", "/v1/work-items", body)
	if got.Code != 429 || !strings.Contains(got.Body.String(), "QUOTA_EXCEEDED") {
		t.Fatal(got.Code, got.Body.String())
	}

	if _, err := s.Store.DB.Exec(`DELETE FROM workspaces WHERE id='ws_quota_0'`); err != nil {
		t.Fatal(err)
	}
	got = request(t, s, "POST", "/v1/work-items", body)
	if got.Code != 201 {
		t.Fatal(got.Code, got.Body.String())
	}
	got = request(t, s, "POST", "/v1/work-items", body)
	if got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
}
