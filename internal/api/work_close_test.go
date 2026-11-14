package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestCloseWorkWithoutApproval(t *testing.T) {
	s, _ := setup(t)
	project := domainProject(t, s, "close-review")
	var item shared.WorkItem
	created := request(t, s, "POST", "/v1/work-items", `{"project_id":"`+project.ID+`","instruction":"Review this example","kind":"research","idempotency_key":"close-example"}`)
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &item) != nil {
		t.Fatal(created.Code, created.Body.String())
	}
	root := "/v1/work-items/" + item.ID
	run := request(t, s, "POST", root+"/run", `{"project_id":"`+project.ID+`","idempotency_key":"first"}`)
	if run.Code != 202 {
		t.Fatal(run.Code, run.Body.String())
	}
	item = waitWorkReview(t, s, project.ID, item.ID)
	for _, extra := range []string{``, `,"expected_attempt_id":"attempt_stale"`, `,"accepted":true,"expected_attempt_id":"` + item.LatestAttemptID + `"`} {
		got := request(t, s, "POST", root+"/review", `{"project_id":"`+project.ID+`","decision":"closed"`+extra+`}`)
		if got.Code != 400 && got.Code != 409 {
			t.Fatal("invalid close accepted", got.Code, got.Body.String())
		}
	}
	body := `{"project_id":"` + project.ID + `","decision":"closed","expected_attempt_id":"` + item.LatestAttemptID + `"}`
	for i := 0; i < 2; i++ {
		got := request(t, s, "POST", root+"/review", body)
		if got.Code != 200 {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	repo := orchestration.Repository{DB: s.Store.DB}
	stored, err := repo.Work(context.Background(), project.ID, item.ID)
	if err != nil || stored.State != "closed" || stored.LatestAttemptID != item.LatestAttemptID {
		t.Fatal(stored, err)
	}
	attempts, err := repo.Attempts(context.Background(), project.ID, item.ID)
	if err != nil || len(attempts) != 1 || attempts[0].ReviewState != "closed" {
		t.Fatal(attempts, err)
	}
	if store.BoardStage(stored.State) != "done" {
		t.Fatal("closed work is not done")
	}
	got := request(t, s, "POST", root+"/review", `{"project_id":"`+project.ID+`","accepted":true,"expected_attempt_id":"`+item.LatestAttemptID+`"}`)
	if got.Code != 409 {
		t.Fatal("closed work silently approved", got.Code)
	}
	// Legacy/manual placement must not contradict the saved review outcome.
	if err := s.Store.SaveBoardItem(context.Background(), store.BoardItem{
		ID: item.ID, ProjectID: project.ID, Stage: "running", ExecutionState: "closed",
	}, 0); err != nil {
		t.Fatal(err)
	}
	board := request(t, s, "GET", "/v1/board?project_id="+project.ID, "")
	var listed struct {
		Items []boardCard `json:"items"`
	}
	if board.Code != 200 || json.Unmarshal(board.Body.Bytes(), &listed) != nil || len(listed.Items) != 1 || listed.Items[0].Stage != "done" || listed.Items[0].Work.State != "closed" {
		t.Fatal("closed task has conflicting board status", board.Body.String())
	}
	var moved *httptest.ResponseRecorder
	for i, stage := range []string{"archived", "done"} {
		moved = request(t, s, "POST", "/v1/board/"+item.ID, fmt.Sprintf(`{"project_id":%q,"revision":%d,"stage":%q}`, project.ID, i+1, stage))
		if moved.Code != 200 {
			t.Fatal("archive/restore failed", moved.Body.String())
		}
	}
	stored, err = repo.Work(context.Background(), project.ID, item.ID)
	if err != nil || stored.State != "closed" {
		t.Fatal("archive changed review outcome", stored, err)
	}

	// A concurrent board edit must roll back work, attempt, and history changes.
	candidate, err := s.Store.BoardItem(context.Background(), project.ID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Stage = "running"
	if _, err = s.Store.MoveBoardWork(context.Background(), candidate, candidate.Revision-1, stored, s.now()); err != store.ErrBoardConflict {
		t.Fatal("missing revision conflict", err)
	}
	unchanged, err := repo.Work(context.Background(), project.ID, item.ID)
	if err != nil || unchanged.State != "closed" {
		t.Fatal("partial reopen committed", unchanged, err)
	}
	var historyCount int
	if err = s.Store.DB.QueryRow(`SELECT count(*) FROM work_review_history WHERE work_item_id=$1`, item.ID).Scan(&historyCount); err != nil || historyCount != 0 {
		t.Fatal("partial history committed", historyCount, err)
	}
	for i, test := range []struct{ stage, state string }{{"planned", "planned"}, {"running", "in_progress"}, {"review", "awaiting_review"}} {
		moved = request(t, s, "POST", "/v1/board/"+item.ID, fmt.Sprintf(`{"project_id":%q,"revision":%d,"stage":%q}`, project.ID, i+3, test.stage))
		var card boardCard
		if moved.Code != 200 || json.Unmarshal(moved.Body.Bytes(), &card) != nil || card.Stage != test.stage || card.Work.State != test.state {
			t.Fatal("reopen disagrees with task", moved.Code, moved.Body.String())
		}
		attempts, err = repo.Attempts(context.Background(), project.ID, item.ID)
		if err != nil || len(attempts) != 1 || attempts[0].ID != item.LatestAttemptID || attempts[0].ReviewState != "" {
			t.Fatal("reopen dispatched or retained current decision", attempts, err)
		}
	}
	var retained string
	if err = s.Store.DB.QueryRow(`SELECT decision FROM work_review_history WHERE work_item_id=$1`, item.ID).Scan(&retained); err != nil || retained != "closed" {
		t.Fatal("closure history lost", retained, err)
	}
	stale := request(t, s, "POST", "/v1/board/"+item.ID, fmt.Sprintf(`{"project_id":%q,"revision":3,"stage":"running"}`, project.ID))
	if stale.Code != 409 {
		t.Fatal("stale move accepted", stale.Code)
	}
	approved := request(t, s, "POST", root+"/review", fmt.Sprintf(`{"project_id":%q,"accepted":true,"expected_attempt_id":%q}`, project.ID, item.LatestAttemptID))
	if approved.Code != 200 {
		t.Fatal("reopened result cannot be reviewed", approved.Body.String())
	}
	moved = request(t, s, "POST", "/v1/board/"+item.ID, fmt.Sprintf(`{"project_id":%q,"revision":6,"stage":"running"}`, project.ID))
	if moved.Code != 200 {
		t.Fatal("accepted work cannot reopen", moved.Body.String())
	}
	var decisions int
	if err = s.Store.DB.QueryRow(`SELECT count(*) FROM work_review_history WHERE work_item_id=$1`, item.ID).Scan(&decisions); err != nil || decisions != 2 {
		t.Fatal("decision history", decisions, err)
	}
	continued := request(t, s, "POST", root+"/continue", fmt.Sprintf(`{"project_id":%q,"expected_attempt_id":%q,"instruction":"Improve the result","idempotency_key":"after-reopen"}`, project.ID, item.LatestAttemptID))
	if continued.Code != 200 && continued.Code != 202 {
		t.Fatal("reopened task cannot continue", continued.Code, continued.Body.String())
	}

}
