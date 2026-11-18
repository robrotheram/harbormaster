package orchestration

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	"errors"
	_ "modernc.org/sqlite"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkReopenAndProjectScopedKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "work.db")
	var saved shared.WorkItem
	for i := 0; i < 2; i++ {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		repo := Repository{DB: db}
		if _, err = repo.DB.ExecContext(ctx, Schema); err != nil {
			t.Fatal(err)
		}
		input := shared.WorkItem{ProjectID: "project_a", WorkspaceID: "workspace_a", SpecificationPath: "task.json", SpecificationDigest: strings.Repeat("a", 64), Kind: "research", CreatedAt: time.Now()}
		item, created, err := repo.CreateWork(ctx, input, "same-key")
		if err != nil || created != (i == 0) {
			t.Fatal(created, err)
		}
		if i == 0 {
			saved = item
		} else if item.ID != saved.ID || !item.CreatedAt.Equal(saved.CreatedAt) {
			t.Fatal("retry rewrote metadata")
		}
		input.ProjectID = "project_b"
		other, _, err := repo.CreateWork(ctx, input, "same-key")
		if err != nil || other.ID == item.ID {
			t.Fatal("cross-project collision", err)
		}
		if _, err = repo.Work(ctx, "project_b", item.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Fatal("foreign work exposed", err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
