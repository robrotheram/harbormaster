package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

// Each execution owns a schema, never the operator's existing tables. CI sets
// TEST_DATABASE_URL to an ephemeral PostgreSQL service; ordinary tests use SQLite.
func TestPostgresMetadataContract(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to exercise PostgreSQL persistence")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("open PostgreSQL test connection:", err)
	}
	defer admin.Close()
	schema := shared.NewID("test_")
	if _, err = admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal("create isolated test schema:", err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.ExecContext(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("remove isolated test schema:", err)
		}
	}()
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal("migrate PostgreSQL metadata:", err)
	}
	defer func() {
		if s != nil {
			s.DB.Close()
		}
	}()
	var count, version int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*), max(version) FROM schema_migrations`).Scan(&count, &version); err != nil || count != 2 || version != 2 {
		t.Fatal("expected PostgreSQL schema version 2", count, version, err)
	}

	now := time.Now().UTC()
	project := shared.Project{ID: "project_test", Name: "Test project", CreatedAt: now}
	if err = s.CreateProject(ctx, project, "create-project"); err != nil {
		t.Fatal(err)
	}
	w := shared.Workspace{ID: "ws_test", ProjectID: project.ID, HomeID: "home_test", AgentKind: "codex", State: "STOPPED", CreatedAt: now, UpdatedAt: now}
	if err = s.Create(ctx, w, "create-workspace"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ByProjectKey(ctx, project.ID, "create-workspace"); err != nil || got.ID != w.ID {
		t.Fatal("project-scoped creation key did not round-trip", err)
	}
	forged := w
	forged.ProjectID = "default"
	if err = s.Save(ctx, forged); !errors.Is(err, ErrOwnership) {
		t.Fatal("workspace ownership mutation accepted", err)
	}

	profile, err := s.ConnectionProfile(ctx, "github")
	if err != nil {
		t.Fatal(err)
	}
	expected := profile.Revision
	profile.Name = "Edited in PostgreSQL"
	profile.Revision++
	if err = s.SaveConnectionProfile(ctx, profile, expected); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveConnectionProfile(ctx, profile, expected); !errors.Is(err, ErrProfileConflict) {
		t.Fatal("stale profile revision accepted", err)
	}

	repo := orchestration.Repository{DB: s.DB}
	schedule := shared.Schedule{ID: "schedule_test", ProjectID: project.ID, WorkspaceID: w.ID, Name: "Test schedule", Provider: "codex", InstructionPath: "task.md", RunAt: now, NextRunAt: now, CreatedAt: now, Enabled: true}
	if _, created, err := repo.CreateSchedule(ctx, schedule, "create-schedule"); err != nil || !created {
		t.Fatal("schedule creation failed", err)
	}
	first, err := repo.Enqueue(ctx, schedule, "occurrence-1", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.Enqueue(ctx, schedule, "occurrence-1", now.Add(time.Second))
	if err != nil || first.ID != second.ID || !first.CreatedAt.Equal(second.CreatedAt) {
		t.Fatal("occurrence retry changed durable identity", err)
	}
	if err := repo.DeliverDue(ctx, schedule.ID, now); err != nil {
		t.Fatal("transactional PostgreSQL delivery failed", err)
	}
	if err := repo.DeliverDue(ctx, schedule.ID, now.Add(time.Second)); err != nil {
		t.Fatal("due delivery retry failed", err)
	}
	if got, err := repo.Schedule(ctx, schedule.ID); err != nil || got.Enabled {
		t.Fatal("one-off schedule was not advanced atomically", err)
	}
	if runs, err := repo.Runs(ctx, project.ID); err != nil || len(runs) != 2 {
		t.Fatal("PostgreSQL due delivery duplicated or lost an occurrence", err)
	}

	if err = s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, u.String())
	if err != nil {
		t.Fatal("reopen PostgreSQL metadata:", err)
	}
	if got, err := s.ConnectionProfile(ctx, profile.ID); err != nil || got.Name != profile.Name || got.Revision != profile.Revision {
		t.Fatal("startup replaced edited profile", err)
	}
	if got, err := s.Workspace(ctx, w.ID); err != nil || got.HomeID != w.HomeID || got.ProjectID != project.ID {
		t.Fatal("workspace ownership did not survive reopen", err)
	}
}
