package taskstore

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func repository(t *testing.T) (*Repository, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	r := New(db, "workspace_one")
	if err := r.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r, db
}

func TestFileAcceptanceFailureRollsBackTaskReservation(t *testing.T) {
	r, db := repository(t)
	ctx := context.Background()
	if _, err := db.Exec(`CREATE TRIGGER reject_file_acceptance BEFORE INSERT ON file_acceptances BEGIN SELECT RAISE(ABORT,'simulated storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	s := Submission{Key: "occurrence_one", Instruction: "task", Provider: "codex", SourcePath: "instructions.md"}
	if _, err := r.Accept(ctx, s, time.Now().UTC()); err == nil {
		t.Fatal("failed file acceptance acknowledged")
	}
	if active, err := r.ActiveCount(ctx); err != nil || active != 0 {
		t.Fatal("failed acceptance left runnable task", active, err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_file_acceptance`); err != nil {
		t.Fatal(err)
	}
	accepted, err := r.Accept(ctx, s, time.Now().UTC())
	if err != nil || !accepted.Created {
		t.Fatal("retry after storage repair failed", err)
	}
	if _, err := r.FileAcceptance(ctx, s.Key, "other.md", s.Provider, "", ""); !errors.Is(err, ErrConflict) {
		t.Fatal("changed file identity accepted", err)
	}
}

func TestRestartPreservesAcceptanceAndInterruptsWithoutReplay(t *testing.T) {
	r, db := repository(t)
	ctx := context.Background()
	s := Submission{Key: "occurrence_one", Instruction: "task", Provider: "codex", SourcePath: "instructions.md"}
	first, err := r.Accept(ctx, s, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(ctx, first.Task.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	restarted := New(db, "workspace_one")
	if err := restarted.Init(ctx); err != nil {
		t.Fatal(err)
	}
	replay, err := restarted.Accept(ctx, s, time.Now().UTC())
	if err != nil || replay.Created || replay.Task.ID != first.Task.ID || replay.Task.State != "failed" || replay.Task.ErrorCode != "INTERRUPTED" {
		t.Fatal("restart replayed execution or lost acceptance", replay, err)
	}
	byFile, err := restarted.FileAcceptance(ctx, s.Key, s.SourcePath, s.Provider, "", "")
	if err != nil || byFile.ID != first.Task.ID {
		t.Fatal("restart lost file acceptance", byFile, err)
	}
}

func TestDigestReceiptAndLegacyMigration(t *testing.T) {
	r, db := repository(t)
	ctx := context.Background()
	s := Submission{Key: "digest-one", Instruction: "original", Provider: "codex", SourcePath: "instructions.md", SpecificationDigest: "abc"}
	a, err := r.Accept(ctx, s, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := r.FileAcceptance(ctx, s.Key, s.SourcePath, s.Provider, "abc", "")
	if err != nil || receipt.ID != a.Task.ID {
		t.Fatalf("%+v %v", receipt, err)
	}
	if _, err = r.FileAcceptance(ctx, s.Key, s.SourcePath, s.Provider, "def", ""); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err = r.FileAcceptance(ctx, s.Key, s.SourcePath, s.Provider, "", ""); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err = db.Exec(`ALTER TABLE file_acceptances DROP COLUMN specification_digest`); err != nil {
		t.Fatal(err)
	}
	if err = r.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = r.FileAcceptance(ctx, s.Key, s.SourcePath, s.Provider, "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestSelectedModelSurvivesRestartAndConflictsOnReplay(t *testing.T) {
	r, db := repository(t)
	ctx := context.Background()
	s := Submission{Key: "model-choice", Instruction: "hello", Provider: "codex", Model: "chosen-model", SourcePath: "request.md", SpecificationDigest: "abc"}
	first, err := r.Accept(ctx, s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	restarted := New(db, "workspace_one")
	if err := restarted.Init(ctx); err != nil {
		t.Fatal(err)
	}
	receipt, err := restarted.FileAcceptance(ctx, s.Key, s.SourcePath, s.Provider, s.SpecificationDigest, s.Model)
	if err != nil || receipt.ID != first.Task.ID {
		t.Fatal(receipt, err)
	}
	for _, model := range []string{"", "other-model"} {
		if _, err := restarted.FileAcceptance(ctx, s.Key, s.SourcePath, s.Provider, s.SpecificationDigest, model); !errors.Is(err, ErrConflict) {
			t.Fatal("changed receipt accepted", err)
		}
		s.Model = model
		if _, err := restarted.Accept(ctx, s, time.Now()); !errors.Is(err, ErrConflict) {
			t.Fatal("changed submission accepted", err)
		}
	}
}
