package orchestration

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type workLifecycle struct{}

func (workLifecycle) Refresh(context.Context, *shared.Workspace) error { return nil }
func (workLifecycle) Start(context.Context, *shared.Workspace) error   { return nil }

type workAgent struct {
	lost          bool
	keys          []string
	failArtifacts bool
	cancelled     bool
}

func (a *workAgent) Agent(_ context.Context, ws, method, path string, b []byte) (int, []byte, error) {
	if path == "/v1/tasks/from-file" {
		var in map[string]string
		json.Unmarshal(b, &in)
		if in["specification_digest"] != strings.Repeat("a", 64) {
			return 400, nil, nil
		}
		a.keys = append(a.keys, in["idempotency_key"])
		if !a.lost {
			a.lost = true
			return 0, nil, errors.New("lost acknowledgement")
		}
		b, _ = json.Marshal(shared.Task{ID: "task_one", WorkspaceID: ws, State: "running"})
		return 202, b, nil
	}
	if strings.HasSuffix(path, "/cancel") {
		a.cancelled = true
		return 200, []byte(`{}`), nil
	}
	if path == "/v1/artifacts" {
		if method == "POST" {
			var publication map[string]string
			json.Unmarshal(b, &publication)
			if publication["source_path"] != "result.md" || !strings.HasPrefix(publication["idempotency_key"], "work:attempt_") {
				return 400, nil, nil
			}
			if a.failArtifacts {
				return 503, nil, nil
			}
			return 201, []byte(`{"id":"artifact_one"}`), nil
		}
		return 500, nil, errors.New("global artifact enumeration forbidden")
	}
	state := "succeeded"
	if a.cancelled {
		state = "cancelled"
	}
	b, _ = json.Marshal(shared.Task{ID: "task_one", WorkspaceID: ws, State: state})
	return 200, b, nil
}
func TestWorkExecutionRecoveryReviewAndRetry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "work.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	repo := Repository{DB: db}
	for _, init := range []func(context.Context) error{repo.Init, repo.InitWork, repo.InitWorkAttempts} {
		if err = init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	item, _, err := repo.CreateWork(ctx, shared.WorkItem{ProjectID: "project_a", WorkspaceID: "ws_a", ArtifactPaths: []string{"result.md"}, SpecificationPath: "work.json", SpecificationDigest: strings.Repeat("a", 64), Kind: "research", CreatedAt: time.Now()}, "create")
	if err != nil {
		t.Fatal(err)
	}
	first, err := repo.RunWork(ctx, item.ProjectID, item.ID, "run", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := repo.RunWork(ctx, item.ProjectID, item.ID, "run", time.Now())
	if err != nil || duplicate.ID != first.ID {
		t.Fatal("delivery duplicated", err)
	}
	if _, err = repo.RunWork(ctx, item.ProjectID, item.ID, "parallel", time.Now()); !errors.Is(err, ErrConflict) {
		t.Fatal("parallel writer admitted", err)
	}
	if _, err = repo.Attempts(ctx, "project_b", item.ID); err != sql.ErrNoRows {
		t.Fatal("foreign attempts exposed", err)
	}
	agent := &workAgent{}
	ws := &recoveringWorkspaces{workspace: shared.Workspace{ID: "ws_a", ProjectID: "project_a"}}
	scheduler := Scheduler{Cursor: &Cursor{}, Repository: repo, Workspaces: ws, Lifecycle: workLifecycle{}, Agent: agent, Now: time.Now, Lock: func(...string) func() { return func() {} }}
	if err = scheduler.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo.DB = db
	scheduler.Repository = repo
	for i := 0; i < 2; i++ {
		if err = scheduler.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(agent.keys) != 2 || agent.keys[0] != agent.keys[1] {
		t.Fatal("recovery changed acceptance key", agent.keys)
	}
	stored, err := repo.Work(ctx, item.ProjectID, item.ID)
	if err != nil || stored.State != "awaiting_review" {
		t.Fatal(stored, err)
	}
	attempts, err := repo.Attempts(ctx, item.ProjectID, item.ID)
	if err != nil || len(attempts) != 1 || len(attempts[0].Artifacts) != 1 || !strings.Contains(attempts[0].Artifacts[0].ContentURL, first.WorkItemID) {
		t.Fatal(attempts, err)
	}
	if _, err = repo.ReviewWork(ctx, item.ProjectID, item.ID, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	retry, err := repo.RunWork(ctx, item.ProjectID, item.ID, "intentional-retry", time.Now())
	if err != nil || retry.ID == first.ID {
		t.Fatal(retry, err)
	}
	for i := 0; i < 2; i++ {
		if i == 1 {
			agent.failArtifacts = true
		}
		if err = scheduler.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = repo.ReviewWork(ctx, item.ProjectID, item.ID, true, time.Now()); !errors.Is(err, ErrConflict) {
		t.Fatal("failed publication accepted", err)
	}
	agent.failArtifacts = false
	third, err := repo.RunWork(ctx, item.ProjectID, item.ID, "third", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CancelWork(ctx, item.ProjectID, item.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	attempts, err = repo.Attempts(ctx, item.ProjectID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range attempts {
		if a.ID == third.ID && a.State != "cancelled" {
			t.Fatal(a)
		}
	}
}

func TestAttemptStorageFailureIsNotReportedAsStateConflict(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "storage-failure.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	repo := Repository{DB: db}
	for _, init := range []func(context.Context) error{repo.InitWork, repo.InitWorkAttempts} {
		if err = init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	item, _, err := repo.CreateWork(ctx, shared.WorkItem{ProjectID: "project", WorkspaceID: "workspace", SpecificationPath: "work.md", SpecificationDigest: strings.Repeat("a", 64), Kind: "research", CreatedAt: time.Now()}, "create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TRIGGER fail_attempt BEFORE INSERT ON work_attempts BEGIN SELECT RAISE(ABORT,'synthetic storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.RunWork(ctx, item.ProjectID, item.ID, "attempt", time.Now()); err == nil || errors.Is(err, ErrConflict) {
		t.Fatal("storage failure misclassified", err)
	}
	stored, err := repo.Work(ctx, item.ProjectID, item.ID)
	if err != nil || stored.State != "draft" || stored.LatestAttemptID != "" {
		t.Fatal("failed transaction changed work", stored, err)
	}
}

type retryRejectedWorkAgent struct {
	workAgent
	status     int
	rejected   bool
	deliveries []string
}

func (a *retryRejectedWorkAgent) Agent(ctx context.Context, workspace, method, path string, body []byte) (int, []byte, error) {
	if path == "/v1/tasks/from-file" {
		var request map[string]string
		if err := json.Unmarshal(body, &request); err != nil {
			return 0, nil, err
		}
		a.deliveries = append(a.deliveries, request["idempotency_key"])
		if a.lost && !a.rejected {
			a.rejected = true
			return a.status, []byte(`{"code":"WORKSPACE_BUSY"}`), nil
		}
	}
	return a.workAgent.Agent(ctx, workspace, method, path, body)
}
func TestUncertainDispatchCancellationSurvivesRetryableRejection(t *testing.T) {
	for _, status := range []int{409, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			ctx := context.Background()
			db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "cancel-recovery.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := Repository{DB: db}
			for _, init := range []func(context.Context) error{repo.Init, repo.InitWork, repo.InitWorkAttempts} {
				if err = init(ctx); err != nil {
					t.Fatal(err)
				}
			}
			item, _, err := repo.CreateWork(ctx, shared.WorkItem{ProjectID: "project_a", WorkspaceID: "ws_a", SpecificationPath: "work.md", SpecificationDigest: strings.Repeat("a", 64), Kind: "software", CreatedAt: time.Now()}, "create")
			if err != nil {
				t.Fatal(err)
			}
			attempt, err := repo.RunWork(ctx, item.ProjectID, item.ID, "run", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			agent := &retryRejectedWorkAgent{status: status}
			scheduler := Scheduler{Repository: repo, Cursor: &Cursor{}, Workspaces: &recoveringWorkspaces{workspace: shared.Workspace{ID: item.WorkspaceID, ProjectID: item.ProjectID}}, Lifecycle: workLifecycle{}, Agent: agent, Now: time.Now, Lock: func(...string) func() { return func() {} }}
			// The engine durably accepts work but its acknowledgement is lost.
			if err = scheduler.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err = repo.CancelWork(ctx, item.ProjectID, item.ID, time.Now()); err != nil {
				t.Fatal(err)
			}
			// A retryable rejection cannot turn the cancellation into normal queued work.
			if err = scheduler.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			attempts, err := repo.Attempts(ctx, item.ProjectID, item.ID)
			if err != nil || len(attempts) != 1 || attempts[0].State != "cancel_requested" {
				t.Fatalf("cancellation lost after HTTP %d: %+v %v", status, attempts, err)
			}
			// Receipt recovery returns the original task, which must then be cancelled.
			for i := 0; i < 2; i++ {
				if err = scheduler.Tick(ctx); err != nil {
					t.Fatal(err)
				}
			}
			attempts, err = repo.Attempts(ctx, item.ProjectID, item.ID)
			if err != nil || len(attempts) != 1 || attempts[0].ID != attempt.ID || attempts[0].State != "cancelled" || !agent.cancelled {
				t.Fatalf("recovered task not cancelled: %+v %v", attempts, err)
			}
			if len(agent.deliveries) != 3 {
				t.Fatal("unexpected dispatch deliveries", agent.deliveries)
			}
			for _, key := range agent.deliveries {
				if key != "work:"+attempt.ID {
					t.Fatal("recovery changed acceptance identity", key)
				}
			}
		})
	}
}
