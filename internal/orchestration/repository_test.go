package orchestration

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	"errors"
	_ "modernc.org/sqlite"
	"path/filepath"
	"testing"
	"time"
)

func TestDueDeliveryRollsBackBothOccurrenceAndTrigger(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "scheduler.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := Repository{DB: db}
	if err = repo.Init(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	schedule := shared.Schedule{ID: "schedule_atomic", ProjectID: "project_atomic", WorkspaceID: "ws_atomic", Enabled: true, NextRunAt: now.Add(-time.Minute), RunAt: now.Add(-time.Minute)}
	if _, _, err = repo.CreateSchedule(ctx, schedule, "create"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TRIGGER fail_schedule_update BEFORE UPDATE ON schedules BEGIN SELECT RAISE(ABORT,'synthetic update failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = repo.DeliverDue(ctx, schedule.ID, now); err == nil {
		t.Fatal("fault-injected delivery succeeded")
	}
	runs, err := repo.Runs(ctx, "")
	if err != nil || len(runs) != 0 {
		t.Fatalf("occurrence escaped rolled-back trigger update: %v %v", runs, err)
	}
	saved, err := repo.Schedule(ctx, schedule.ID)
	if err != nil || !saved.Enabled {
		t.Fatal("trigger changed after failed delivery", err)
	}
	if _, err = db.Exec(`DROP TRIGGER fail_schedule_update`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = repo.DeliverDue(ctx, schedule.ID, now); err != nil {
			t.Fatal(err)
		}
	}
	runs, err = repo.Runs(ctx, "")
	if err != nil || len(runs) != 1 {
		t.Fatalf("retry duplicated or lost occurrence: %v %v", runs, err)
	}
	if !errors.Is(repo.SaveRun(ctx, shared.Run{ID: "run_missing", ProjectID: schedule.ProjectID}), sql.ErrNoRows) {
		t.Fatal("missing run update reported success")
	}
	if !errors.Is(repo.SaveSchedule(ctx, shared.Schedule{ID: schedule.ID, ProjectID: "foreign"}), sql.ErrNoRows) {
		t.Fatal("foreign schedule update reported success")
	}
}
func TestAdvanceDueCoalescesMissedIntervalsAndKeepsOccurrenceIdentity(t *testing.T) {
	due := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	original := shared.Schedule{ID: "schedule_intervals", ProjectID: "project_intervals", Enabled: true, NextRunAt: due, IntervalSeconds: 60}
	next, run := AdvanceDue(original, due.Add(3*time.Minute+30*time.Second))
	if run == nil || !next.NextRunAt.Equal(due.Add(4*time.Minute)) {
		t.Fatal("missed occurrence policy changed")
	}
	_, retry := AdvanceDue(original, due.Add(9*time.Minute))
	if retry == nil || retry.ID != run.ID {
		t.Fatal("recovery changed occurrence identity")
	}
	if _, run = AdvanceDue(next, due.Add(3*time.Minute+30*time.Second)); run != nil {
		t.Fatal("future occurrence delivered early")
	}
}
