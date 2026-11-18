package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

// The real metadata store/API/lifecycle adapter are exercised. Only native CLI
// acceptance is replaced: these tests do not call a provider or claim VM/network
// isolation. The acceptance map represents the workspace's durable idempotency.
type scheduledRuntime struct {
	*embeddedRuntime
	acceptanceMu         sync.Mutex
	tasks                map[string]shared.Task
	instructionBodies    []string
	dispatchKeys         []string
	acknowledgementsLost int
}

func (r *scheduledRuntime) Agent(ctx context.Context, id, method, path string, body []byte) (int, []byte, error) {
	if method == "POST" && path == "/v1/tasks/from-file" {
		var input struct {
			Path     string `json:"instruction_path"`
			Provider string `json:"provider"`
			Key      string `json:"idempotency_key"`
		}
		if err := json.Unmarshal(body, &input); err != nil {
			return 400, nil, err
		}
		if !shared.ValidProjectFile(input.Path) || input.Provider != "codex" || input.Key == "" {
			return 400, nil, errors.New("invalid native dispatch")
		}
		var fields map[string]any
		json.Unmarshal(body, &fields)
		if _, exists := fields["instruction"]; exists {
			return 400, nil, errors.New("platform transmitted a prompt body")
		}
		r.acceptanceMu.Lock()
		defer r.acceptanceMu.Unlock()
		r.dispatchKeys = append(r.dispatchKeys, input.Key)
		key := id + "\x00" + input.Key
		task, exists := r.tasks[key]
		if !exists {
			content, err := os.ReadFile(filepath.Join(r.base, id, "files", input.Path))
			if err != nil {
				return 404, []byte(`{"code":"FILE_UNAVAILABLE"}`), nil
			}
			r.instructionBodies = append(r.instructionBodies, string(content))
			task = shared.Task{ID: shared.NewID("task_"), WorkspaceID: id, State: "queued", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			r.tasks[key] = task
		}
		if r.acknowledgementsLost > 0 {
			r.acknowledgementsLost--
			return 0, nil, errors.New("lost acknowledgement after durable acceptance")
		}
		reply, _ := json.Marshal(task)
		if exists {
			return 200, reply, nil
		}
		return 202, reply, nil
	}
	if method == "GET" && strings.HasPrefix(path, "/v1/tasks/") {
		taskID := strings.TrimPrefix(path, "/v1/tasks/")
		r.acceptanceMu.Lock()
		defer r.acceptanceMu.Unlock()
		for _, task := range r.tasks {
			if task.WorkspaceID == id && task.ID == taskID {
				reply, _ := json.Marshal(task)
				return 200, reply, nil
			}
		}
	}
	return r.embeddedRuntime.Agent(ctx, id, method, path, body)
}
func schedulerSetup(t *testing.T) (*Server, *scheduledRuntime) {
	t.Helper()
	s, base := setup(t)
	runtime := &scheduledRuntime{embeddedRuntime: base, tasks: map[string]shared.Task{}}
	s.Runtime = runtime
	return s, runtime
}
func scheduleRequestBody(project, workspace, key string, runAt time.Time) string {
	b, _ := json.Marshal(map[string]any{"project_id": project, "workspace_id": workspace, "name": "Project maintenance", "instruction_path": "jobs/task.md", "provider": "codex", "run_at": runAt, "interval_seconds": 0, "idempotency_key": key})
	return string(b)
}
func createSchedule(t *testing.T, s *Server, token, project, workspace, key string, runAt time.Time) shared.Schedule {
	t.Helper()
	w := domainRequest(t, s, token, "POST", "/v1/schedules", scheduleRequestBody(project, workspace, key, runAt))
	if w.Code != 201 && w.Code != 200 {
		t.Fatalf("schedule create: %d %s", w.Code, w.Body)
	}
	var schedule shared.Schedule
	if json.Unmarshal(w.Body.Bytes(), &schedule) != nil || schedule.ID == "" {
		t.Fatal("schedule identity missing")
	}
	return schedule
}
func TestSchedulerProjectCredentialsFenceCreateListsAndActions(t *testing.T) {
	s, rt := schedulerSetup(t)
	a, b := domainProject(t, s, "scheduler-A"), domainProject(t, s, "scheduler-B")
	s.cfg.ProjectTokens = map[string]string{"project-A-access": a.ID, "project-B-access": b.ID}
	wa := domainWorkspace(t, s, "control-token", a.ID, "wa")
	wb := domainWorkspace(t, s, "control-token", b.ID, "wb")
	future := s.now().Add(time.Hour)
	sa := createSchedule(t, s, "project-A-access", a.ID, wa.ID, "same-key", future)
	sb := createSchedule(t, s, "project-B-access", b.ID, wb.ID, "same-key", future)
	if sa.ID == sb.ID {
		t.Fatal("schedule idempotency collided across projects")
	}
	if createSchedule(t, s, "project-A-access", a.ID, wa.ID, "same-key", future).ID != sa.ID {
		t.Fatal("schedule retry duplicated metadata")
	}
	for _, token := range []string{"project-A-access", "control-token"} {
		w := domainRequest(t, s, token, "POST", "/v1/schedules", scheduleRequestBody(a.ID, wb.ID, "forbidden-binding", future))
		if w.Code != 404 {
			t.Fatal("schedule workspace crossed project")
		}
	}
	if w := domainRequest(t, s, "project-A-access", "POST", "/v1/schedules", scheduleRequestBody(b.ID, wb.ID, "forged-project", future)); w.Code != 404 {
		t.Fatal("schedule project argument widened authority")
	}
	for _, action := range []string{"enable", "disable", "run-now"} {
		w := domainRequest(t, s, "project-A-access", "POST", "/v1/schedules/"+sb.ID+"/"+action, `{"idempotency_key":"denied"}`)
		if w.Code != 404 {
			t.Fatalf("foreign schedule %s accepted: %d", action, w.Code)
		}
	}
	for _, route := range []string{"/v1/schedules", "/v1/runs"} {
		if w := domainRequest(t, s, "project-A-access", "GET", route+"?project_id="+b.ID, ""); w.Code != 404 {
			t.Fatal("foreign project list filter accepted")
		}
	}
	w := domainRequest(t, s, "project-A-access", "GET", "/v1/schedules", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), sb.ID) || strings.Contains(w.Body.String(), b.ID) || !strings.Contains(w.Body.String(), sa.ID) {
		t.Fatal("schedule list leaked another project")
	}
	var first, again shared.Run
	for i, out := range []*shared.Run{&first, &again} {
		w := domainRequest(t, s, "project-A-access", "POST", "/v1/schedules/"+sa.ID+"/run-now", `{"idempotency_key":"one-manual-occurrence"}`)
		if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), out) != nil {
			t.Fatalf("manual occurrence %d failed", i)
		}
	}
	if first.ID == "" || first.ID != again.ID {
		t.Fatal("manual retry created another occurrence")
	}
	w = domainRequest(t, s, "project-B-access", "POST", "/v1/schedules/"+sb.ID+"/run-now", `{"idempotency_key":"one-manual-occurrence"}`)
	if w.Code != 202 {
		t.Fatal("own manual occurrence denied")
	}
	for _, route := range []string{"/v1/runs", "/v1/schedules"} {
		w = domainRequest(t, s, "project-A-access", "GET", route, "")
		if w.Code != 200 || strings.Contains(w.Body.String(), b.ID) || strings.Contains(w.Body.String(), wb.ID) {
			t.Fatal("foreign operational metadata leaked")
		}
	}
	repo := orchestration.Repository{DB: s.Store.DB}
	runs, err := repo.Runs(context.Background(), a.ID)
	if err != nil || len(runs) != 1 {
		t.Fatal("manual retries persisted duplicate occurrences")
	}
	if rt.starts != 0 {
		t.Fatal("HTTP schedule metadata operations started runtime before dispatch")
	}
}
func writeScheduledInstruction(t *testing.T, rt *scheduledRuntime, workspace, content string) {
	t.Helper()
	dir := filepath.Join(rt.base, workspace, "files/jobs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func assertNoInstructionInMetadata(t *testing.T, s *Server, marker string) {
	t.Helper()
	for _, table := range []string{"projects", "workspaces", "task_metadata", "schedules", "orchestration_runs"} {
		var count int
		query := fmt.Sprintf(`SELECT count(*) FROM %s WHERE metadata LIKE $1`, table)
		if err := s.Store.DB.QueryRow(query, "%"+marker+"%").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("instruction content persisted in %s", table)
		}
	}
}
func TestDueScheduleWakesStoppedWorkspaceAndKeepsPromptWorkspaceLocal(t *testing.T) {
	s, rt := schedulerSetup(t)
	project := domainProject(t, s, "scheduled")
	ws := domainWorkspace(t, s, "control-token", project.ID, "scheduled-home")
	const marker = "PRIVATE_SCHEDULE_INSTRUCTION_SENTINEL"
	writeScheduledInstruction(t, rt, ws.ID, marker)
	schedule := createSchedule(t, s, "control-token", project.ID, ws.ID, "due-once", s.now().Add(-time.Second))
	if rt.starts != 0 {
		t.Fatal("schedule creation woke workspace")
	}
	s.tickOrchestration(context.Background())
	if rt.starts != 1 || len(rt.tasks) != 1 || len(rt.instructionBodies) != 1 || rt.instructionBodies[0] != marker {
		t.Fatal("due occurrence did not wake and dispatch workspace-local instruction")
	}
	repo := orchestration.Repository{DB: s.Store.DB}
	runs, err := repo.Runs(context.Background(), project.ID)
	if err != nil || len(runs) != 1 || runs[0].TaskID == "" || runs[0].State != "queued" {
		t.Fatalf("accepted run metadata missing: %+v %v", runs, err)
	}
	stored, err := repo.Schedule(context.Background(), schedule.ID)
	if err != nil || stored.Enabled || stored.LastRunAt == nil {
		t.Fatal("one-off schedule did not finish trigger delivery")
	}
	rt.acceptanceMu.Lock()
	for key, task := range rt.tasks {
		task.State = "succeeded"
		rt.tasks[key] = task
	}
	rt.acceptanceMu.Unlock()
	s.tickOrchestration(context.Background())
	runs, err = repo.Runs(context.Background(), project.ID)
	if err != nil || len(runs) != 1 || runs[0].State != "succeeded" || len(rt.dispatchKeys) != 1 {
		t.Fatal("terminal run was not observed, or one-off replayed")
	}
	assertNoInstructionInMetadata(t, s, marker)
}
func TestLostScheduleAcknowledgementRetriesSameAcceptanceAfterControlRestart(t *testing.T) {
	s, rt := schedulerSetup(t)
	project := domainProject(t, s, "lost-ack")
	ws := domainWorkspace(t, s, "control-token", project.ID, "lost-ack-home")
	const marker = "PRIVATE_LOST_ACK_INSTRUCTION_SENTINEL"
	writeScheduledInstruction(t, rt, ws.ID, marker)
	createSchedule(t, s, "control-token", project.ID, ws.ID, "due-lost-ack", s.now().Add(-time.Second))
	rt.acknowledgementsLost = 1
	s.tickOrchestration(context.Background())
	repo := orchestration.Repository{DB: s.Store.DB}
	runs, err := repo.Runs(context.Background(), project.ID)
	if err != nil || len(runs) != 1 || runs[0].State != "dispatching" || runs[0].TaskID != "" || len(rt.tasks) != 1 {
		t.Fatal("lost acknowledgement was treated as unaccepted or completed")
	}
	// Rebuild API state while keeping only the database and workspace acceptance.
	restored := New(s.Store, rt, s.cfg)
	restored.now = s.now
	restored.tickOrchestration(context.Background())
	runs, err = repo.Runs(context.Background(), project.ID)
	if err != nil || len(runs) != 1 || runs[0].TaskID == "" || runs[0].State != "queued" {
		t.Fatal("control-plane recovery did not find accepted native task")
	}
	if len(rt.tasks) != 1 || len(rt.instructionBodies) != 1 || len(rt.dispatchKeys) != 2 || rt.dispatchKeys[0] != rt.dispatchKeys[1] {
		t.Fatal("lost acknowledgement duplicated accepted execution or changed its durable key")
	}
	restored.tickOrchestration(context.Background())
	if len(rt.dispatchKeys) != 2 {
		t.Fatal("known native task was resubmitted")
	}
	assertNoInstructionInMetadata(t, s, marker)
}

func TestHermesScheduleProviderRequiresHermesWorkspace(t *testing.T) {
	s, base := setup(t)
	project := domainProject(t, s, "hermes-schedules")
	s.Runtime = &hermesRuntime{embeddedRuntime: base}
	config := shared.HermesSetup{ProjectID: project.ID, Provider: "openai-codex", Mode: "workspace", Model: "approved-model"}
	configBody, _ := json.Marshal(config)
	if got := request(t, s, "POST", "/v1/projects/"+project.ID+"/hermes", string(configBody)); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	created := request(t, s, "POST", "/v1/workspaces", `{"project_id":"`+project.ID+`","agent_kind":"hermes","idempotency_key":"hermes-schedule-home"}`)
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	var hermes shared.Workspace
	json.Unmarshal(created.Body.Bytes(), &hermes)
	other := domainWorkspace(t, s, "control-token", project.ID, "other-home")
	for _, check := range []struct {
		workspace string
		status    int
	}{{hermes.ID, 201}, {other.ID, 400}} {
		body, _ := json.Marshal(map[string]any{"project_id": project.ID, "workspace_id": check.workspace, "name": "Daily digest", "instruction_path": "jobs/digest-task.md", "provider": "hermes", "run_at": time.Now().Add(time.Hour), "interval_seconds": 86400, "idempotency_key": "schedule-" + check.workspace})
		got := request(t, s, "POST", "/v1/schedules", string(body))
		if got.Code != check.status {
			t.Fatal(got.Code, got.Body.String())
		}
	}
}
