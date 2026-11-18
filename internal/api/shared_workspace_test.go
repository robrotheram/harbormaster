package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSharedWorkspaceReusedAcrossProjectsAndRetries(t *testing.T) {
	s, rt := setup(t)
	s.cfg.SharedWorkspaceID = "ws_shared"
	projects := []string{}
	for _, name := range []string{"One", "Two"} {
		response := request(t, s, "POST", "/v1/projects", fmt.Sprintf(`{"name":%q,"idempotency_key":%q}`, name, name))
		var project shared.Project
		if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &project) != nil {
			t.Fatal(response.Code, response.Body)
		}
		projects = append(projects, project.ID)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			response := request(t, s, "POST", "/v1/workspaces", fmt.Sprintf(`{"project_id":%q,"idempotency_key":"create-%d"}`, projects[i%2], i))
			var ws shared.Workspace
			if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &ws) != nil || ws.ID != "ws_shared" || !ws.Shared {
				t.Errorf("shared create: %d %s", response.Code, response.Body)
			}
		}(i)
	}
	wg.Wait()
	items, err := s.Store.Workspaces(context.Background())
	if err != nil || len(items) != 1 || rt.starts != 0 {
		t.Fatalf("created extra compute or metadata: %v %v", items, err)
	}
	for _, project := range projects {
		response := request(t, s, "GET", "/v1/workspaces?project_id="+project, "")
		var list struct {
			Items []shared.Workspace `json:"items"`
		}
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &list) != nil || len(list.Items) != 1 || list.Items[0].ID != "ws_shared" || !list.Items[0].Shared {
			t.Fatalf("new project cannot select shared workspace: %d %s", response.Code, response.Body)
		}
	}
	if response := request(t, s, "GET", "/v1/agent-homes", ""); response.Code != 404 {
		t.Fatal("retired home catalog remains available", response.Code)
	}
	for _, body := range []string{`{"idempotency_key":"branch","parent_id":"ws_shared","branch":"feature"}`, `{"idempotency_key":"other","agent_kind":"codex"}`} {
		if response := request(t, s, "POST", "/v1/workspaces", body); response.Code != 409 {
			t.Fatal("created another worker", response.Code)
		}
	}
	if response := request(t, s, "POST", "/v1/workspaces/ws_shared/delete", `{"confirm_id":"ws_shared"}`); response.Code != 409 {
		t.Fatal("shared worker deletion admitted", response.Code)
	}
	if response := request(t, s, "POST", "/v1/projects/"+projects[0]+"/delete", fmt.Sprintf(`{"confirm_id":%q}`, projects[0])); response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
	if _, err := s.Store.Workspace(context.Background(), "ws_shared"); err != nil {
		t.Fatal("project deletion removed shared workspace", err)
	}
}

func TestSharedExecutionDoesNotBroadenProjectMetadataAuthorization(t *testing.T) {
	s, _ := setup(t)
	s.cfg.SharedWorkspaceID = "ws_shared"
	for _, id := range []string{"project_a", "project_b"} {
		if err := s.Store.CreateProject(context.Background(), shared.Project{ID: id, Name: id, CreatedAt: s.now()}, id); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := s.EnsureSharedWorkspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !s.workspaceInProject(ws, "project_a") || !s.workspaceInProject(ws, "project_b") {
		t.Fatal("shared execution unavailable")
	}
	if s.workspaceInProject(shared.Workspace{ID: "ws_foreign", ProjectID: "project_a", Shared: true}, "project_b") {
		t.Fatal("trusted persisted sharing flag instead of configured identity")
	}
	s.cfg.ProjectTokens = map[string]string{"project-a-token": "project_a"}
	call := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer project-a-token")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if got := call("/v1/workspaces/ws_shared"); got.Code != 200 {
		t.Fatal("explicitly shared workspace unavailable", got.Code, got.Body)
	}
	if got := call("/v1/workspaces?project_id=project_b"); got.Code != 404 {
		t.Fatal("foreign metadata filter admitted", got.Code)
	}
	if got := call("/v1/projects"); got.Code != 200 || strings.Contains(got.Body.String(), "project_b") {
		t.Fatal("foreign project metadata leaked", got.Code, got.Body)
	}
}

func TestSharedModeCannotStartDedicatedWorker(t *testing.T) {
	s, rt := setup(t)
	dedicated := create(t, s, "dedicated")
	s.cfg.SharedWorkspaceID = "ws_shared"
	if err := s.lifecycle().Start(context.Background(), &dedicated); err == nil || err.Error() != "SHARED_WORKSPACE_ONLY" || rt.starts != 0 {
		t.Fatal("dedicated execution admitted", err)
	}
}

func TestSharedInlineWorkAndSchedulesKeepLogicalProjects(t *testing.T) {
	s, rt := schedulerSetup(t)
	s.cfg.SharedWorkspaceID = "ws_shared"
	if _, err := s.EnsureSharedWorkspace(context.Background()); err != nil {
		t.Fatal(err)
	}
	projects := []shared.Project{domainProject(t, s, "shared-a"), domainProject(t, s, "shared-b")}
	paths := map[string]bool{}
	for _, project := range projects {
		body := fmt.Sprintf(`{"project_id":%q,"instruction":%q,"kind":"software","idempotency_key":"same-key"}`, project.ID, "Work for "+project.Name)
		response := request(t, s, "POST", "/v1/work-items", body)
		var item shared.WorkItem
		if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &item) != nil {
			t.Fatal(response.Code, response.Body)
		}
		if item.ProjectID != project.ID || item.WorkspaceID != "ws_shared" || paths[item.SpecificationPath] {
			t.Fatal("work lost logical project or reused another instruction", item)
		}
		paths[item.SpecificationPath] = true
		if response = request(t, s, "POST", "/v1/work-items", body); response.Code != 200 {
			t.Fatal("work retry failed", response.Code, response.Body)
		}
	}
	if err := os.MkdirAll(filepath.Join(rt.base, "ws_shared", "files", "jobs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt.base, "ws_shared", "files", "jobs", "task.md"), []byte("shared scheduled task"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, project := range projects {
		createSchedule(t, s, "control-token", project.ID, "ws_shared", "schedule", s.now().Add(-time.Second))
	}
	s.tickOrchestration(context.Background())
	if len(rt.dispatchKeys) != 2 {
		t.Fatalf("schedules did not dispatch through shared membership: %v", rt.dispatchKeys)
	}
	items, err := s.Store.Workspaces(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatal("automatic work allocated another workspace", items, err)
	}
}

func TestSharedModePreservesStoppedDedicatedWorkerMetadata(t *testing.T) {
	s, _ := setup(t)
	dedicated := create(t, s, "dedicated")
	s.cfg.SharedWorkspaceID = "ws_shared"
	if _, err := s.EnsureSharedWorkspace(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Reconcile(context.Background())
	after, err := s.Store.Workspace(context.Background(), dedicated.ID)
	if err != nil || after.State != "STOPPED" || !after.UpdatedAt.Equal(dedicated.UpdatedAt) {
		t.Fatal("reconciled archived worker", after, err)
	}
	dedicated.State = "READY"
	dedicated.Ready = true
	if err := s.Store.Save(context.Background(), dedicated); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureSharedWorkspace(context.Background()); err == nil {
		t.Fatal("shared mode accepted another running worker")
	}
}

func TestDeletedSharedProjectCannotDispatchQueuedOrFutureWork(t *testing.T) {
	s, rt := schedulerSetup(t)
	s.cfg.SharedWorkspaceID = "ws_shared"
	ctx := context.Background()
	if _, err := s.EnsureSharedWorkspace(ctx); err != nil {
		t.Fatal(err)
	}
	project := domainProject(t, s, "deleted-shared")
	schedule := createSchedule(t, s, "control-token", project.ID, "ws_shared", "scheduled", s.now().Add(-time.Second))
	repo := orchestration.Repository{DB: s.Store.DB}
	if _, err := repo.Enqueue(ctx, schedule, "queued", s.now()); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"project_id":%q,"instruction":"retained draft","kind":"software","idempotency_key":"draft"}`, project.ID)
	response := request(t, s, "POST", "/v1/work-items", body)
	var item shared.WorkItem
	if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &item) != nil {
		t.Fatal(response.Code, response.Body)
	}
	response = request(t, s, "POST", "/v1/work-items/"+item.ID+"/run", fmt.Sprintf(`{"project_id":%q,"idempotency_key":"run"}`, project.ID))
	if response.Code != 202 {
		t.Fatal(response.Code, response.Body)
	}
	// Deletion cannot race a busy or unobservable shared runtime.
	for _, unknown := range []bool{false, true} {
		rt.jobs, rt.unknown = 1, unknown
		response = request(t, s, "POST", "/v1/projects/"+project.ID+"/delete", fmt.Sprintf(`{"confirm_id":%q}`, project.ID))
		if response.Code != 409 {
			t.Fatal("busy deletion admitted", response.Code, response.Body)
		}
	}
	rt.jobs, rt.unknown = 0, false
	response = request(t, s, "POST", "/v1/projects/"+project.ID+"/delete", fmt.Sprintf(`{"confirm_id":%q}`, project.ID))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
	for i := 0; i < 2; i++ {
		s.tickOrchestration(ctx)
	}
	if len(rt.dispatchKeys) != 0 {
		t.Fatal("deleted project dispatched", rt.dispatchKeys)
	}
	runs, err := repo.Runs(ctx, project.ID)
	if err != nil || len(runs) != 1 || runs[0].State != "failed" {
		t.Fatal("future or queued run admitted", runs, err)
	}
	attempts, err := repo.Attempts(ctx, project.ID, item.ID)
	if err != nil || len(attempts) != 1 || attempts[0].State != "failed" {
		t.Fatal("queued work admitted", attempts, err)
	}
	if _, err := s.Store.Workspace(ctx, "ws_shared"); err != nil {
		t.Fatal("deleted shared worker", err)
	}
}
