package api

import (
	"agent-workbench/internal/shared"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSampleProjectPlansTasksWithoutExecution(t *testing.T) {
	for _, sharedMode := range []bool{false, true} {
		t.Run(fmt.Sprintf("shared=%t", sharedMode), func(t *testing.T) {
			s, rt := setup(t)
			if sharedMode {
				s.cfg.SharedWorkspaceID = "ws_shared"
			}
			body := `{"name":"Sample project","idempotency_key":"sample-test","sample":true}`
			res := humanRequest(t, s, "alice@example.com", "POST", "/v1/projects", body)
			var project shared.Project
			if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &project) != nil || !project.Sample {
				t.Fatal(res.Code, res.Body)
			}
			res = humanRequest(t, s, "alice@example.com", "GET", "/v1/board?project_id="+project.ID, "")
			var list struct {
				Items []boardCard `json:"items"`
			}
			if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &list) != nil || len(list.Items) != 3 {
				t.Fatal(res.Code, res.Body)
			}
			ids := map[string]bool{}
			for _, card := range list.Items {
				ids[card.ID] = true
			}
			dependencies := 0
			for _, card := range list.Items {
				if card.Title == "" || card.Description == nil || *card.Description == "" || card.Priority == "" || card.Owner != "" || card.Stage != "planned" || card.Work.State != "planned" || card.Work.WorkspaceID != "" || card.Work.LatestAttemptID != "" || card.Work.SpecificationPath != "" {
					t.Fatalf("invalid planning card: %+v", card)
				}
				for _, dep := range card.Dependencies {
					if !ids[dep] {
						t.Fatal("foreign dependency", dep)
					}
					dependencies++
				}
			}
			if dependencies != 2 {
				t.Fatal("missing sample dependencies")
			}
			// Retrying creation must neither duplicate tasks nor overwrite user edits.
			card := list.Items[0]
			card.Title = "My edited task"
			if err := s.Store.SaveBoardItem(t.Context(), card.BoardItem, card.Revision); err != nil {
				t.Fatal(err)
			}
			res = humanRequest(t, s, "alice@example.com", "POST", "/v1/projects", body)
			var retry shared.Project
			if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &retry) != nil || retry.ID != project.ID {
				t.Fatal(res.Code, res.Body)
			}
			saved, err := s.Store.BoardItem(t.Context(), project.ID, card.ID)
			if err != nil || saved.Title != "My edited task" {
				t.Fatal("retry overwrote edit", saved, err)
			}
			var count int
			if err := s.Store.DB.QueryRow(`SELECT COUNT(*) FROM work_items WHERE project_id=$1`, project.ID).Scan(&count); err != nil || count != 3 {
				t.Fatal(count, err)
			}
			if !sharedMode {
				res = request(t, s, "POST", "/v1/work-items/"+card.ID+"/run", fmt.Sprintf(`{"project_id":%q,"idempotency_key":"run-plan"}`, project.ID))
				if res.Code != 409 {
					t.Fatal("unprepared plan dispatched", res.Code, res.Body)
				}
			}
			if err := s.Store.DB.QueryRow(`SELECT COUNT(*) FROM work_attempts`).Scan(&count); err != nil || count != 0 {
				t.Fatal("sample queued work", count, err)
			}
			workers, err := s.Store.Workspaces(t.Context())
			if err != nil || len(workers) != 0 || rt.starts != 0 {
				t.Fatal("sample started compute", workers, rt.starts, err)
			}
			res = humanRequest(t, s, "outsider@example.com", "GET", "/v1/board?project_id="+project.ID, "")
			if res.Code != 404 {
				t.Fatal("sample exposed to non-member", res.Code, res.Body)
			}
			res = humanRequest(t, s, "alice@example.com", "POST", "/v1/projects", `{"name":"Sample project","idempotency_key":"sample-test"}`)
			if res.Code != 409 {
				t.Fatal("sample flag not checked on retry", res.Code, res.Body)
			}
		})
	}
}

func TestSampleProjectCreationRollsBackIfTasksFail(t *testing.T) {
	s, _ := setup(t)
	// Fail after one task has been inserted, proving the entire sample is atomic.
	_, err := s.Store.DB.Exec(`CREATE TRIGGER fail_sample BEFORE INSERT ON board_items WHEN NEW.metadata LIKE '%Add task search and filters%' BEGIN SELECT RAISE(ABORT, 'test failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	res := request(t, s, "POST", "/v1/projects", `{"name":"Sample project","idempotency_key":"rollback-sample","sample":true}`)
	if res.Code != 503 {
		t.Fatal(res.Code, res.Body)
	}
	if _, err := s.Store.ProjectByKey(t.Context(), "rollback-sample"); err == nil {
		t.Fatal("partial project retained")
	}
	for _, table := range []string{"work_items", "board_items"} {
		var count int
		if err := s.Store.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("partial sample retained", table, count, err)
		}
	}
}
