package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type resourceRuntime struct {
	*embeddedRuntime
	memory int64
	starts int
}

func (rt *resourceRuntime) Capabilities(context.Context) (protocol.RuntimeCapability, error) {
	return protocol.RuntimeCapability{SchemaVersion: 1, ID: protocol.DevelopmentTemplate, Version: "test", AgentKinds: []string{"t3", "codex"}, NetworkModes: []string{"blocked", "allowlist", "open"}, Resources: protocol.ResourceLimits{CPUCores: 1, MemoryBytes: rt.memory}}, nil
}
func (rt *resourceRuntime) Start(ctx context.Context, id string) (shared.Observation, error) {
	rt.starts++
	return rt.embeddedRuntime.Start(ctx, id)
}
func TestResourceMismatchFailsBeforeQuotaReservationOrCompute(t *testing.T) {
	s, base := setup(t)
	ws := create(t, s, "resource-template")
	rt := &resourceRuntime{embeddedRuntime: base, memory: 2 << 30}
	s.Runtime = rt
	path := "/v1/workspaces/" + ws.ID + "/start"
	w := request(t, s, "POST", path, `{"idempotency_key":"resource-mismatch"}`)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "RESOURCE_LIMIT_UNAVAILABLE") {
		t.Fatal(w.Code, w.Body.String())
	}
	saved, err := s.Store.Workspace(context.Background(), ws.ID)
	if err != nil || saved.State != "STOPPED" || rt.starts != 0 {
		t.Fatal("mismatch allocated compute or reserved quota", saved.State, rt.starts, err)
	}
	rt.memory = ws.MemoryLimitBytes
	w = request(t, s, "POST", path, `{"idempotency_key":"resource-matched"}`)
	if w.Code != 200 || rt.starts != 1 {
		t.Fatal("approved template did not start", w.Code, w.Body.String())
	}
}

func TestTaskPollingDoesNotRenewExplicitOpen(t *testing.T) {
	s, _ := setup(t)
	ws := create(t, s, "poll-policy")
	path := "/v1/workspaces/" + ws.ID
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	if w := request(t, s, "POST", path+"/start", `{"idempotency_key":"open"}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	before, err := s.Store.Workspace(context.Background(), ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(20 * time.Minute)
	for _, suffix := range []string{"", "/result"} {
		_ = request(t, s, "GET", path+"/tasks/task_missing"+suffix, "")
	}
	after, err := s.Store.Workspace(context.Background(), ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.LastOpenedAt == nil || after.LastOpenedAt == nil || !before.LastOpenedAt.Equal(*after.LastOpenedAt) {
		t.Fatal("task polling renewed explicit open")
	}
}

type blockedInspectRuntime struct {
	*embeddedRuntime
	blocked          string
	entered, release chan struct{}
}

func (rt *blockedInspectRuntime) Inspect(ctx context.Context, id string) (shared.Observation, error) {
	if id == rt.blocked {
		select {
		case rt.entered <- struct{}{}:
		default:
		}
		select {
		case <-rt.release:
		case <-ctx.Done():
			return shared.Observation{}, ctx.Err()
		}
	}
	return rt.embeddedRuntime.Inspect(ctx, id)
}
func TestSlowWorkspaceDoesNotBlockIndependentReadsAndStart(t *testing.T) {
	s, base := setup(t)
	a, b := create(t, s, "slow-A"), create(t, s, "fast-B")
	rt := &blockedInspectRuntime{embeddedRuntime: base, blocked: a.ID, entered: make(chan struct{}, 1), release: make(chan struct{})}
	s.Runtime = rt
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(rt.release) }) }
	defer release()
	slow := make(chan struct{})
	go func() {
		defer close(slow)
		request(t, s, "POST", "/v1/workspaces/"+a.ID+"/start", `{"idempotency_key":"slow-start"}`)
	}()
	select {
	case <-rt.entered:
	case <-time.After(time.Second):
		t.Fatal("slow request did not enter runtime")
	}
	independent := make(chan error, 1)
	go func() {
		w := request(t, s, "POST", "/v1/workspaces/"+b.ID+"/start", `{"idempotency_key":"independent-start"}`)
		if w.Code != 200 {
			independent <- fmt.Errorf("independent start: %s", w.Body)
			return
		}
		snapshot := httptest.NewRecorder()
		s.DashboardHandler().ServeHTTP(snapshot, httptest.NewRequest("GET", "/dashboard-api/v1/snapshot", nil))
		if snapshot.Code != 200 {
			independent <- fmt.Errorf("snapshot: %s", snapshot.Body)
			return
		}
		independent <- nil
	}()
	select {
	case err := <-independent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("unrelated work blocked behind slow runtime")
	}
	// Let the first request finish before cleanup touches the runtime/store.
	release()
	<-slow
}

func TestSchedulerRotatesPastUnavailablePendingWindow(t *testing.T) {
	s, rt := schedulerSetup(t)
	project := domainProject(t, s, "fairness")
	a, b := domainWorkspace(t, s, "control-token", project.ID, "old"), domainWorkspace(t, s, "control-token", project.ID, "new")
	future := s.now().Add(time.Hour)
	old := createSchedule(t, s, "control-token", project.ID, a.ID, "old", future)
	next := createSchedule(t, s, "control-token", project.ID, b.ID, "new", future)
	writeScheduledInstruction(t, rt, b.ID, "new runnable work")
	repo := orchestration.Repository{DB: s.Store.DB}
	for i := 0; i < 100; i++ {
		run := shared.Run{ID: fmt.Sprintf("run_%032d", i), ProjectID: project.ID, WorkspaceID: a.ID, ScheduleID: old.ID, TaskID: "task_unreachable", State: "running", CreatedAt: s.now(), UpdatedAt: s.now()}
		raw, _ := json.Marshal(run)
		if _, err := s.Store.DB.Exec(`INSERT INTO orchestration_runs(id,project_id,schedule_id,state,created_at,metadata) VALUES($1,$2,$3,$4,$5,$6)`, run.ID, run.ProjectID, run.ScheduleID, run.State, run.CreatedAt.Format(time.RFC3339Nano), string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	target, err := repo.Enqueue(context.Background(), next, "new", s.now())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		s.tickOrchestration(context.Background())
	}
	runs, err := repo.Runs(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		if run.ID == target.ID {
			if run.TaskID == "" {
				t.Fatal("new occurrence starved behind unavailable pending window")
			}
			return
		}
	}
	t.Fatal("target run missing")
}

func TestSnapshotUsesExplicitPublicWorkspaceFields(t *testing.T) {
	s, _ := setup(t)
	ws := create(t, s, "snapshot-fields")
	ws.Connections = &shared.ConnectionPolicy{Mode: "allowlist", Destinations: []string{"private.example:443"}}
	ws.ConnectionProfileID = "sensitive-profile"
	ws.ConsoleEnabled = true
	if err := s.Store.Save(context.Background(), ws); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.DashboardHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard-api/v1/snapshot", nil))
	for _, private := range []string{"home_id", "agent_kind", "connection_profile_id", "connections", "console_enabled", "last_opened_at", "private.example"} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatalf("snapshot published %s", private)
		}
	}
	var value struct {
		Workspaces struct {
			Items []map[string]json.RawMessage `json:"items"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if len(value.Workspaces.Items) != 1 || len(value.Workspaces.Items[0]) != 17 {
		t.Fatalf("unexpected public workspace fields: %s", w.Body.String())
	}
}

type blockedStartRuntime struct {
	*embeddedRuntime
	blocked          string
	entered, release chan struct{}
}

func (rt *blockedStartRuntime) Start(ctx context.Context, id string) (shared.Observation, error) {
	if id == rt.blocked {
		close(rt.entered)
		select {
		case <-rt.release:
		case <-ctx.Done():
			return shared.Observation{}, ctx.Err()
		}
	}
	return rt.embeddedRuntime.Start(ctx, id)
}
func TestStartingWorkspaceReservesQuotaBeforeRuntimeCall(t *testing.T) {
	s, base := setup(t)
	a, b := create(t, s, "reserved-A"), create(t, s, "reserved-B")
	s.cfg.MaxRunning = 1
	rt := &blockedStartRuntime{embeddedRuntime: base, blocked: a.ID, entered: make(chan struct{}), release: make(chan struct{})}
	s.Runtime = rt
	var once sync.Once
	release := func() { once.Do(func() { close(rt.release) }) }
	defer release()
	done := make(chan struct{})
	go func() {
		defer close(done)
		request(t, s, "POST", "/v1/workspaces/"+a.ID+"/start", `{"idempotency_key":"reserve"}`)
	}()
	select {
	case <-rt.entered:
	case <-time.After(time.Second):
		t.Fatal("start did not reach runtime")
	}
	w := request(t, s, "POST", "/v1/workspaces/"+b.ID+"/start", `{"idempotency_key":"over-quota"}`)
	if w.Code != 429 {
		t.Fatalf("concurrent start bypassed reservation: %d %s", w.Code, w.Body)
	}
	release()
	<-done
}

func TestProjectAssistantWorkflowCreatesOnceAndFencesProjects(t *testing.T) {
	s, base := setup(t)
	project := domainProject(t, s, "assistant-project")
	other := domainProject(t, s, "other-project")
	s.Runtime = &hermesRuntime{embeddedRuntime: base}
	config := shared.HermesSetup{ProjectID: project.ID, Provider: "openai-codex", Mode: "workspace", Model: "approved-model", ConnectionProfileID: "blocked"}
	raw, _ := json.Marshal(config)
	path := "/v1/projects/" + project.ID + "/hermes"
	if w := request(t, s, "POST", path, string(raw)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var first shared.Workspace
	for i := 0; i < 2; i++ {
		w := request(t, s, "POST", path+"/assistant", `{"idempotency_key":"assistant-once"}`)
		want := 200
		if i == 0 {
			want = 201
		}
		var ws shared.Workspace
		if w.Code != want || json.Unmarshal(w.Body.Bytes(), &ws) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		if i == 0 {
			first = ws
		} else if ws.ID != first.ID {
			t.Fatal("assistant duplicated")
		}
	}
	if first.ProjectID != project.ID || first.AgentKind != "hermes" || first.ConnectionProfileID != "blocked" || !first.Ready {
		t.Fatalf("incomplete assistant: %+v", first)
	}
	s.cfg.ProjectTokens = map[string]string{"other-project-credential": other.ID}
	denied := domainRequest(t, s, "other-project-credential", "POST", path+"/assistant", `{"idempotency_key":"denied"}`)
	if denied.Code != 404 {
		t.Fatal("cross-project assistant operation admitted")
	}
}

func TestProjectAssistantRefusesAmbiguousExistingWorkspaces(t *testing.T) {
	s, base := setup(t)
	project := domainProject(t, s, "ambiguous-assistant")
	s.Runtime = &hermesRuntime{embeddedRuntime: base}
	config := shared.HermesSetup{ProjectID: project.ID, Provider: "openai-codex", Mode: "workspace", Model: "approved-model"}
	raw, _ := json.Marshal(config)
	path := "/v1/projects/" + project.ID + "/hermes"
	if w := request(t, s, "POST", path, string(raw)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for i := 0; i < 2; i++ {
		payload := fmt.Sprintf(`{"project_id":%q,"agent_kind":"hermes","idempotency_key":"candidate-%d"}`, project.ID, i)
		if w := request(t, s, "POST", "/v1/workspaces", payload); w.Code != 201 {
			t.Fatal(w.Body.String())
		}
	}
	w := request(t, s, "POST", path+"/assistant", `{"idempotency_key":"ambiguous"}`)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "ASSISTANT_WORKSPACE_AMBIGUOUS") {
		t.Fatal("ambiguous selection accepted", w.Code, w.Body.String())
	}
	if base.starts != 0 {
		t.Fatal("ambiguous selection started a workspace")
	}
}
