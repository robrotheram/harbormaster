package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitWorkReview(t *testing.T, s *Server, project, id string) shared.WorkItem {
	t.Helper()
	repo := orchestration.Repository{DB: s.Store.DB}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		s.tickOrchestration(context.Background())
		item, err := repo.Work(context.Background(), project, id)
		if err != nil {
			t.Fatal(err)
		}
		if item.State == "awaiting_review" {
			return item
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("work did not reach review")
	return shared.WorkItem{}
}

func TestWorkConversationFollowupPreservesHistoryAndDeduplicates(t *testing.T) {
	s, rt := setup(t)
	project := domainProject(t, s, "conversation")
	foreign := domainProject(t, s, "foreign-conversation")
	var item shared.WorkItem
	created := request(t, s, "POST", "/v1/work-items", `{"project_id":"`+project.ID+`","instruction":"Original private research request","kind":"research","idempotency_key":"conversation"}`)
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &item) != nil {
		t.Fatal(created.Code, created.Body.String())
	}
	root := "/v1/work-items/" + item.ID
	if got := request(t, s, "GET", root+"/instruction?project_id="+project.ID, ""); got.Code != 200 || !strings.Contains(got.Body.String(), "Original private research request") {
		t.Fatal(got.Code, got.Body.String())
	}
	if got := request(t, s, "GET", root+"/instruction?project_id="+foreign.ID, ""); got.Code != 404 {
		t.Fatal("foreign instruction exposed", got.Code)
	}
	if got := request(t, s, "POST", root+"/run", `{"project_id":"`+project.ID+`","idempotency_key":"first-run"}`); got.Code != 202 {
		t.Fatal(got.Code, got.Body.String())
	}
	item = waitWorkReview(t, s, project.ID, item.ID)
	original := item
	if got := request(t, s, "POST", root+"/review", `{"project_id":"`+project.ID+`","accepted":true,"expected_attempt_id":"`+item.LatestAttemptID+`"}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	body := `{"project_id":"` + project.ID + `","instruction":"Add a private comparison experiment","expected_attempt_id":"` + item.LatestAttemptID + `","idempotency_key":"followup-one"}`
	var next shared.WorkAttempt
	for _, status := range []int{202, 200} {
		got := request(t, s, "POST", root+"/continue", body)
		if got.Code != status || json.Unmarshal(got.Body.Bytes(), &next) != nil {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	if next.ParentAttemptID != original.LatestAttemptID || next.WorkspaceID != original.WorkspaceID || next.SpecificationPath == original.SpecificationPath {
		t.Fatal("followup lost provenance", next)
	}
	content, err := os.ReadFile(filepath.Join(rt.base, item.WorkspaceID, "files", next.SpecificationPath))
	if err != nil || !strings.Contains(string(content), original.SpecificationPath) || !strings.Contains(string(content), "Add a private comparison experiment") {
		t.Fatal("private followup missing", err)
	}
	// The new request references a private snapshot containing both sides of
	// the earlier turn, not just the original specification.
	contextFiles, err := filepath.Glob(filepath.Join(rt.base, item.WorkspaceID, "files", "work-requests", "context-*.md"))
	if err != nil || len(contextFiles) != 1 {
		t.Fatal("missing private conversation context", contextFiles, err)
	}
	contextBody, err := os.ReadFile(contextFiles[0])
	if err != nil || !strings.Contains(string(contextBody), "Original private research request") || !strings.Contains(string(contextBody), "# Agent reply\n\nsensitive-result-marker") || !strings.Contains(string(content), filepath.Base(contextFiles[0])) {
		t.Fatal("followup omitted prior conversation", err)
	}
	if got := request(t, s, "POST", root+"/continue", strings.Replace(body, "private comparison", "different comparison", 1)); got.Code != 409 {
		t.Fatal("changed retry accepted", got.Code)
	}
	if got := request(t, s, "POST", root+"/continue", strings.Replace(body, "followup-one", "stale-followup", 1)); got.Code != 409 {
		t.Fatal("stale followup accepted", got.Code)
	}
	if got := request(t, s, "POST", root+"/continue", strings.Replace(body, project.ID, foreign.ID, 1)); got.Code != 404 {
		t.Fatal("foreign followup accepted", got.Code)
	}
	// Instructions and feedback must not leak into central work metadata.
	for _, table := range []string{"work_items", "work_attempts"} {
		rows, err := s.Store.DB.Query(`SELECT metadata FROM ` + table)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var data string
			if err = rows.Scan(&data); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(data, "private comparison") || strings.Contains(data, "Original private") {
				t.Fatal("private text in central metadata")
			}
		}
		rows.Close()
	}
	item = waitWorkReview(t, s, project.ID, item.ID)
	repo := orchestration.Repository{DB: s.Store.DB}
	attempts, err := repo.Attempts(context.Background(), project.ID, item.ID)
	if err != nil || len(attempts) != 2 {
		t.Fatal(attempts, err)
	}
	for _, a := range attempts {
		if a.ID == original.LatestAttemptID && (a.SpecificationPath != original.SpecificationPath || a.ReviewState != "accepted") {
			t.Fatal("earlier accepted result lost", a)
		}
		if a.ID == next.ID {
			next = a
		}
	}
	// The episode filter must exclude the original request on this same worker.
	got := request(t, s, "GET", "/v1/workspaces/"+item.WorkspaceID+"/history?episode="+next.TaskID, "")
	if got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	var snapshot struct {
		Page shared.HistoryPage `json:"page"`
	}
	if json.Unmarshal(got.Body.Bytes(), &snapshot) != nil || len(snapshot.Page.Records) == 0 {
		t.Fatal(got.Body.String())
	}
	for _, record := range snapshot.Page.Records {
		if record.EpisodeID != next.TaskID {
			t.Fatal("unrelated task exposed", record.EpisodeID)
		}
	}
	if got = request(t, s, "GET", "/v1/workspaces/"+item.WorkspaceID+"/history?episode=../bad", ""); got.Code != 400 {
		t.Fatal("invalid episode accepted", got.Code)
	}
	if got = request(t, s, "POST", root+"/review", `{"project_id":"`+project.ID+`","accepted":true,"expected_attempt_id":"`+original.LatestAttemptID+`"}`); got.Code != 409 {
		t.Fatal("stale review accepted", got.Code)
	}
}

type followupLostAck struct {
	*embeddedRuntime
	lose bool
}

func (rt *followupLostAck) Agent(ctx context.Context, id, method, path string, b []byte) (int, []byte, error) {
	status, reply, err := rt.embeddedRuntime.Agent(ctx, id, method, path, b)
	if rt.lose && path == "/v1/files/write-new" && status == 201 {
		rt.lose = false
		return 0, nil, fmt.Errorf("lost file publication acknowledgement")
	}
	return status, reply, err
}
func TestWorkConversationRecoversPrivatePublicationAndRefusesActiveRevision(t *testing.T) {
	s, base := setup(t)
	project := domainProject(t, s, "followup-recovery")
	var item shared.WorkItem
	got := request(t, s, "POST", "/v1/work-items", `{"project_id":"`+project.ID+`","instruction":"Private original","kind":"software","idempotency_key":"initial"}`)
	if got.Code != 201 || json.Unmarshal(got.Body.Bytes(), &item) != nil {
		t.Fatal(got.Code, got.Body.String())
	}
	root := "/v1/work-items/" + item.ID
	got = request(t, s, "POST", root+"/run", `{"project_id":"`+project.ID+`","idempotency_key":"run"}`)
	var first shared.WorkAttempt
	json.Unmarshal(got.Body.Bytes(), &first)
	body := `{"project_id":"` + project.ID + `","instruction":"Private follow-up","expected_attempt_id":"` + first.ID + `","idempotency_key":"revision"}`
	if got = request(t, s, "POST", root+"/continue", body); got.Code != 409 {
		t.Fatal("active revision accepted", got.Code)
	}
	item = waitWorkReview(t, s, project.ID, item.ID)
	s.Runtime = &followupLostAck{embeddedRuntime: base, lose: true}
	if got = request(t, s, "POST", root+"/continue", body); got.Code != 503 {
		t.Fatal("lost acknowledgement not simulated", got.Code)
	}
	if got = request(t, s, "POST", root+"/continue", body); got.Code != 202 {
		t.Fatal("publication retry failed", got.Code, got.Body.String())
	}
	if got = request(t, s, "POST", root+"/continue", body); got.Code != 200 {
		t.Fatal("acceptance retry failed", got.Code, got.Body.String())
	}
	repo := orchestration.Repository{DB: s.Store.DB}
	attempts, err := repo.Attempts(context.Background(), project.ID, item.ID)
	if err != nil || len(attempts) != 2 {
		t.Fatal("duplicate followup", attempts, err)
	}
	for _, a := range attempts {
		if a.ID == first.ID && a.ReviewState != "changes_requested" {
			t.Fatal("feedback decision missing", a)
		}
	}
}
