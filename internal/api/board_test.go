package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func boardWork(t *testing.T, s *Server) shared.WorkItem {
	t.Helper()
	repo := orchestration.Repository{DB: s.Store.DB}
	work, _, err := repo.CreateWork(context.Background(), shared.WorkItem{ProjectID: "default", WorkspaceID: "ws_board", SpecificationPath: "request.md", SpecificationDigest: strings.Repeat("a", 64), Kind: "software", CreatedAt: time.Now()}, "board-test")
	if err != nil {
		t.Fatal(err)
	}
	return work
}
func TestBoardMovesAreDurableAndDoNotRewriteExecution(t *testing.T) {
	s, _ := setup(t)
	work := boardWork(t, s)
	response := request(t, s, "GET", "/v1/board?project_id=default", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"stage":"planned"`) {
		t.Fatal(response.Code, response.Body)
	}
	for i, stage := range []string{"running", "done", "archived", "planned"} {
		response = request(t, s, "POST", "/v1/board/"+work.ID, fmt.Sprintf(`{"project_id":"default","revision":%d,"stage":%q}`, i, stage))
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body)
		}
		stored, err := s.Store.BoardItem(context.Background(), "default", work.ID)
		if err != nil || stored.Stage != stage || stored.Revision != i+1 {
			t.Fatal(stored, err)
		}
	}
	unchanged, err := (orchestration.Repository{DB: s.Store.DB}).Work(context.Background(), "default", work.ID)
	if err != nil || unchanged.State != "draft" {
		t.Fatal("board movement changed execution", err)
	}
	response = request(t, s, "POST", "/v1/board/"+work.ID, `{"project_id":"default","revision":0,"stage":"done"}`)
	if response.Code != 409 {
		t.Fatal("stale revision accepted", response.Code)
	}
	foreign := domainProject(t, s, "foreign-board")
	response = request(t, s, "POST", "/v1/board/"+work.ID, fmt.Sprintf(`{"project_id":%q,"revision":4,"stage":"done"}`, foreign.ID))
	if response.Code != 404 {
		t.Fatal("cross-project change", response.Code)
	}
	response = request(t, s, "POST", "/v1/board/"+work.ID, `{"project_id":"default","revision":4,"stage":"made-up"}`)
	if response.Code != 400 {
		t.Fatal("invalid stage accepted")
	}
}
func TestBoardSyncRoundTripAndConflictProtection(t *testing.T) {
	s, _ := setup(t)
	work := boardWork(t, s)
	remoteTitle, remoteState := "Original issue", "opened"
	remoteBody := "Original description"
	writes := 0
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "synthetic-token" {
			t.Error("missing credential")
		}
		if r.Method == "PUT" {
			var input map[string]string
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				t.Error("bad update")
			}
			writes++
			remoteTitle = input["title"]
			if body, ok := input["description"]; ok {
				remoteBody = body
			}
			remoteState = "opened"
			if input["state_event"] == "close" {
				remoteState = "closed"
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"iid": 12, "title": remoteTitle, "description": remoteBody, "state": remoteState, "web_url": "https://example.test/issues/12"})
	}))
	defer remote.Close()
	prior := http.DefaultTransport
	http.DefaultTransport = remote.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = prior })
	path := filepath.Join(t.TempDir(), "hosts.json")
	b, _ := json.Marshal([]map[string]string{{"name": "office", "provider": "gitlab", "api_url": remote.URL, "token": "synthetic-token"}})
	os.WriteFile(path, b, 0600)
	t.Setenv("WORKBENCH_FORGE_CONFIG", path)
	if err := s.Store.SaveForge(context.Background(), store.ForgeConnection{ID: "forge_sync", Host: "office", Repository: "team/repo"}); err != nil {
		t.Fatal(err)
	}
	card := store.BoardItem{ID: work.ID, ProjectID: "default", Stage: "planned", ExecutionState: "draft", Title: remoteTitle, Source: &store.BoardSource{ConnectionID: "forge_sync", Number: 12, Repository: "team/repo", Title: remoteTitle, State: "open"}}
	if err := s.Store.SaveBoardItem(context.Background(), card, 0); err != nil {
		t.Fatal(err)
	}
	mutate := func(suffix string, body map[string]any, status int) boardCard {
		t.Helper()
		old, err := s.Store.BoardItem(context.Background(), "default", work.ID)
		if err != nil {
			t.Fatal(err)
		}
		body["project_id"] = "default"
		body["revision"] = old.Revision
		payload, _ := json.Marshal(body)
		response := request(t, s, "POST", "/v1/board/"+work.ID+suffix, string(payload))
		if response.Code != status {
			t.Fatal(response.Code, response.Body)
		}
		var out boardCard
		json.Unmarshal(response.Body.Bytes(), &out)
		return out
	}
	mutate("", map[string]any{"stage": "done", "title": "Resolved issue"}, 200)
	result := mutate("/sync", map[string]any{"direction": "push"}, 200)
	if result.Dirty || remoteState != "closed" || remoteTitle != "Resolved issue" || writes != 1 {
		t.Fatal("push failed", result)
	}
	mutate("/sync", map[string]any{"direction": "push"}, 200)
	if writes != 1 {
		t.Fatal("retry rewrote remote")
	}
	remoteTitle = "Edited in GitLab"
	result = mutate("/sync", map[string]any{"direction": "pull"}, 200)
	if result.Title != remoteTitle || result.Dirty {
		t.Fatal("pull failed", result)
	}
	mutate("", map[string]any{"stage": "planned", "title": "Local change"}, 200)
	remoteTitle = "Concurrent remote edit"
	mutate("/sync", map[string]any{"direction": "push"}, 409)
	if writes != 1 || remoteTitle != "Concurrent remote edit" {
		t.Fatal("overwrote concurrent edit")
	}
	mutate("/sync", map[string]any{"direction": "pull"}, 409)
	result = mutate("/sync", map[string]any{"direction": "pull", "discard_local": true}, 200)
	if result.Dirty || result.Title != "Concurrent remote edit" {
		t.Fatal("conflict resolution failed")
	}
	mutate("", map[string]any{"stage": "archived"}, 200)
	result = mutate("/sync", map[string]any{"direction": "push"}, 200)
	if result.Stage != "archived" || writes != 1 {
		t.Fatal("archive changed remote status")
	}
	if remoteBody != "Original description" {
		t.Fatal("title-only sync erased description")
	}
	mutate("", map[string]any{"stage": "planned", "description": "## Expected result\n- [ ] Better search"}, 200)
	result = mutate("/sync", map[string]any{"direction": "push"}, 200)
	if remoteBody != "## Expected result\n- [ ] Better search" || result.Dirty {
		t.Fatal("description sync failed", result)
	}
	mutate("", map[string]any{"stage": "planned", "description": "Local body"}, 200)
	remoteBody = "Remote body"
	priorWrites := writes
	mutate("/sync", map[string]any{"direction": "push"}, 409)
	if writes != priorWrites || remoteBody != "Remote body" {
		t.Fatal("concurrent description overwritten")
	}
	result = mutate("/sync", map[string]any{"direction": "pull", "discard_local": true}, 200)
	if result.Description == nil || *result.Description != "Remote body" || result.Dirty {
		t.Fatal("description pull failed", result)
	}
	mutate("", map[string]any{"stage": "planned", "description": ""}, 200)
	result = mutate("/sync", map[string]any{"direction": "push"}, 200)
	if remoteBody != "" || result.Description == nil || *result.Description != "" || result.Dirty {
		t.Fatal("description clear failed", result)
	}

}

func TestStandaloneBoardIssueCreateAndRetry(t *testing.T) {
	s, _ := setup(t)
	project := domainProject(t, s, "local-board")
	body := `{"project_id":"` + project.ID + `","title":"Improve search","description":"Support partial matches","kind":"software","idempotency_key":"local-issue"}`
	var card boardCard
	for i := 0; i < 2; i++ {
		got := request(t, s, "POST", "/v1/board", body)
		if got.Code != 201 && got.Code != 200 {
			t.Fatal(got.Code, got.Body.String())
		}
		if json.Unmarshal(got.Body.Bytes(), &card) != nil || card.Title != "Improve search" || card.Stage != "planned" || card.Source != nil || card.Work.State != "planned" || card.Work.LatestAttemptID != "" || card.Description == nil || *card.Description != "Support partial matches" {
			t.Fatal(got.Body.String())
		}
	}
	repo := orchestration.Repository{DB: s.Store.DB}
	items, err := repo.WorkItems(context.Background(), project.ID)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	workspaces, err := s.Store.Workspaces(context.Background())
	if err != nil || len(workspaces) != 0 {
		t.Fatal("planning allocated compute", workspaces, err)
	}

	moved := request(t, s, "POST", "/v1/board/"+card.ID, fmt.Sprintf(`{"project_id":%q,"revision":%d,"stage":"archived","title":"New title"}`, project.ID, card.Revision))
	if moved.Code != 200 {
		t.Fatal(moved.Code, moved.Body.String())
	}
	retry := request(t, s, "POST", "/v1/board", body)
	if retry.Code != 200 && retry.Code != 201 {
		t.Fatal(retry.Code, retry.Body.String())
	}
	if !strings.Contains(retry.Body.String(), `"stage":"archived"`) || !strings.Contains(retry.Body.String(), `"title":"New title"`) {
		t.Fatal("retry overwrote edits", retry.Body.String())
	}
	conflict := request(t, s, "POST", "/v1/board", strings.Replace(body, "Improve search", "Another issue", 1))
	if conflict.Code != 409 {
		t.Fatal("changed retry accepted", conflict.Code, conflict.Body.String())
	}
	foreign := domainProject(t, s, "other-board")
	cred, _ := shared.IssueProjectAccess("control-token", foreign.ID, time.Now().Add(time.Hour))
	denied := domainRequest(t, s, cred, "POST", "/v1/board", body)
	if denied.Code != 403 && denied.Code != 404 {
		t.Fatal("foreign creation allowed", denied.Code)
	}
}
