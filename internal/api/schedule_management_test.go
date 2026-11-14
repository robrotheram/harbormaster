package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestScheduleEditingPreservesStateAndRejectsForeignStaleAndActiveChanges(t *testing.T) {
	s, _ := schedulerSetup(t)
	a, b := domainProject(t, s, "schedule-edit-a"), domainProject(t, s, "schedule-edit-b")
	s.cfg.ProjectTokens = map[string]string{"a-access": a.ID, "b-access": b.ID}
	wa := domainWorkspace(t, s, "control-token", a.ID, "old")
	replacement := domainWorkspace(t, s, "control-token", a.ID, "replacement")
	foreign := domainWorkspace(t, s, "control-token", b.ID, "foreign")
	now := s.now()
	schedule := createSchedule(t, s, "a-access", a.ID, wa.ID, "create", now.Add(time.Hour))
	repo := orchestration.Repository{DB: s.Store.DB}
	schedule.Enabled = false
	schedule.LastRunAt = &now
	if err := repo.SaveSchedule(context.Background(), schedule); err != nil {
		t.Fatal(err)
	}
	input := scheduleInput{ProjectID: a.ID, WorkspaceID: replacement.ID, Name: "Updated job", InstructionPath: "jobs/updated.md", Provider: "codex", RunAt: now.Add(2 * time.Hour), Interval: 7200, Key: "edit"}
	post := func(token string, input scheduleInput, want int) shared.Schedule {
		t.Helper()
		body, _ := json.Marshal(input)
		w := domainRequest(t, s, token, "POST", "/v1/schedules/"+schedule.ID+"/update", string(body))
		if w.Code != want {
			t.Fatalf("update: %d %s want %d", w.Code, w.Body, want)
		}
		var out shared.Schedule
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	post("b-access", input, 404)
	bad := input
	bad.WorkspaceID = foreign.ID
	post("a-access", bad, 404)
	bad = input
	bad.InstructionPath = "../private"
	post("a-access", bad, 400)
	updated := post("a-access", input, 200)
	if updated.Revision != 1 || updated.WorkspaceID != replacement.ID || updated.Enabled || updated.LastRunAt == nil || !updated.LastRunAt.Equal(now) || !updated.NextRunAt.Equal(input.RunAt) {
		t.Fatalf("update lost schedule state: %+v", updated)
	}
	// A retry after due delivery must not rewind its trigger or duplicate work.
	updated.NextRunAt = updated.NextRunAt.Add(2 * time.Hour)
	if err := repo.SaveSchedule(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	retry := post("a-access", input, 200)
	if !retry.NextRunAt.Equal(updated.NextRunAt) || retry.Revision != 1 {
		t.Fatal("retry rewound schedule")
	}
	input.Name = "Stale change"
	post("a-access", input, 409)
	input.Revision = 1
	if _, err := repo.Enqueue(context.Background(), updated, "pending", now); err != nil {
		t.Fatal(err)
	}
	post("a-access", input, 409)
}

func TestScheduleDeletionCancelsQueuedAndRetainsAcceptedRunsAndHistory(t *testing.T) {
	s, rt := schedulerSetup(t)
	a, b := domainProject(t, s, "schedule-delete-a"), domainProject(t, s, "schedule-delete-b")
	s.cfg.ProjectTokens = map[string]string{"a-access": a.ID, "b-access": b.ID}
	ws := domainWorkspace(t, s, "control-token", a.ID, "worker")
	now := s.now()
	schedule := createSchedule(t, s, "a-access", a.ID, ws.ID, "create", now.Add(time.Hour))
	repo := orchestration.Repository{DB: s.Store.DB}
	ctx := context.Background()
	queued, _ := repo.Enqueue(ctx, schedule, "queued", now)
	accepted, _ := repo.Enqueue(ctx, schedule, "accepted", now)
	accepted.State = "dispatching"
	if err := repo.SaveRun(ctx, accepted); err != nil {
		t.Fatal(err)
	}
	acceptedQueued, _ := repo.Enqueue(ctx, schedule, "accepted-queued", now)
	acceptedQueued.TaskID = "task_already_accepted"
	if err := repo.SaveRun(ctx, acceptedQueued); err != nil {
		t.Fatal(err)
	}
	history, _ := repo.Enqueue(ctx, schedule, "history", now)
	history.State = "succeeded"
	if err := repo.SaveRun(ctx, history); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token, body string
		status      int
	}{
		{"b-access", `{"idempotency_key":"delete","revision":0}`, 404},
		{"a-access", `{"idempotency_key":"delete","revision":1}`, 409},
		{"a-access", `{"idempotency_key":"delete","revision":0}`, 200},
		{"a-access", `{"idempotency_key":"delete","revision":0}`, 200},
	} {
		w := domainRequest(t, s, tc.token, "POST", "/v1/schedules/"+schedule.ID+"/delete", tc.body)
		if w.Code != tc.status {
			t.Fatalf("delete %d %s want %d", w.Code, w.Body, tc.status)
		}
	}
	items, err := repo.Schedules(ctx, a.ID)
	if err != nil || len(items) != 0 {
		t.Fatal("deleted schedule still listed", err)
	}
	tombstone, err := repo.Schedule(ctx, schedule.ID)
	if err != nil || tombstone.DeletedAt == nil || tombstone.Enabled {
		t.Fatal("schedule not retained and disabled", err)
	}
	if err := repo.DeliverDue(ctx, schedule.ID, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	runs, err := repo.Runs(ctx, a.ID)
	if err != nil || len(runs) != 4 {
		t.Fatal("history lost or new occurrence created", err)
	}
	want := map[string]string{queued.ID: "cancelled", acceptedQueued.ID: "queued", accepted.ID: "dispatching", history.ID: "succeeded"}
	for _, run := range runs {
		if run.State != want[run.ID] {
			t.Fatalf("run changed incorrectly: %+v", run)
		}
	}
	for _, action := range []string{"enable", "disable", "run-now"} {
		w := domainRequest(t, s, "a-access", "POST", "/v1/schedules/"+schedule.ID+"/"+action, `{"idempotency_key":"after-delete"}`)
		if w.Code != 404 {
			t.Fatal("deleted schedule accepted action", action, w.Code)
		}
	}
	if rt.starts != 0 {
		t.Fatal("schedule management unexpectedly started runtime")
	}
}

func TestMissingScheduleWorkerCanBeDisabledAndReplaced(t *testing.T) {
	s, _ := schedulerSetup(t)
	p := domainProject(t, s, "missing-worker")
	ws := domainWorkspace(t, s, "control-token", p.ID, "old")
	schedule := createSchedule(t, s, "control-token", p.ID, ws.ID, "create", s.now().Add(time.Hour))
	// Model the retained schedule left behind when a workspace is deleted.
	repo := orchestration.Repository{DB: s.Store.DB}
	schedule.WorkspaceID = "ws_removed"
	if err := repo.SaveSchedule(context.Background(), schedule); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"enable", "run-now", "disable"} {
		w := domainRequest(t, s, "control-token", "POST", "/v1/schedules/"+schedule.ID+"/"+action, `{"idempotency_key":"missing-worker"}`)
		if action == "disable" {
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
		} else if w.Code != 409 || !strings.Contains(w.Body.String(), "SCHEDULE_WORKSPACE_UNAVAILABLE") {
			t.Fatal(w.Code, w.Body)
		}
	}
	body, _ := json.Marshal(scheduleInput{ProjectID: p.ID, WorkspaceID: ws.ID, Name: schedule.Name, InstructionPath: schedule.InstructionPath, Provider: schedule.Provider, RunAt: schedule.RunAt, Revision: 1, Key: "repair"})
	w := domainRequest(t, s, "control-token", "POST", "/v1/schedules/"+schedule.ID+"/update", string(body))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
}
