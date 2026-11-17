package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type modelCatalogRuntime struct {
	*embeddedRuntime
	foreign bool
}

func (rt *modelCatalogRuntime) Agent(ctx context.Context, id, method, path string, body []byte) (int, []byte, error) {
	if path == "/v1/models" {
		workspace := id
		if rt.foreign {
			workspace = "ws_foreign"
		}
		raw, _ := json.Marshal(shared.AgentModels{WorkspaceID: workspace, DefaultProvider: "codex", DefaultModel: "first-model", Items: []shared.AgentModel{{Provider: "codex", Model: "first-model"}, {Provider: "claude", Model: "second-model"}}})
		return 200, raw, nil
	}
	return rt.embeddedRuntime.Agent(ctx, id, method, path, body)
}
func TestConversationModelSwitchSnapshotsSelectionAndRejectsUnavailableModel(t *testing.T) {
	s, base := setup(t)
	rt := &modelCatalogRuntime{embeddedRuntime: base}
	s.Runtime = rt
	project := domainProject(t, s, "model-switch")
	var item shared.WorkItem
	got := request(t, s, "POST", "/v1/work-items", `{"project_id":"`+project.ID+`","instruction":"Keep my decisions","kind":"research","idempotency_key":"create"}`)
	if got.Code != 201 || json.Unmarshal(got.Body.Bytes(), &item) != nil {
		t.Fatal(got.Code, got.Body.String())
	}
	root := "/v1/work-items/" + item.ID
	if got := request(t, s, "POST", root+"/run", `{"project_id":"`+project.ID+`","idempotency_key":"run"}`); got.Code != 202 {
		t.Fatal(got.Code, got.Body.String())
	}
	item = waitWorkReview(t, s, project.ID, item.ID)
	original := item.LatestAttemptID
	models := "/v1/workspaces/" + item.WorkspaceID + "/models"
	if got := request(t, s, "GET", models, ""); got.Code != 200 || !strings.Contains(got.Body.String(), "second-model") {
		t.Fatal(got.Code, got.Body.String())
	}
	rt.foreign = true
	if got := request(t, s, "GET", models, ""); got.Code != 503 {
		t.Fatal("foreign catalog accepted", got.Code)
	}
	rt.foreign = false
	body := `{"project_id":"` + project.ID + `","instruction":"Continue those decisions","expected_attempt_id":"` + original + `","idempotency_key":"switch","selection":{"provider":"claude","model":"second-model"}}`
	bad := strings.Replace(strings.Replace(body, "second-model", "unknown-model", 1), `"switch"`, `"unavailable"`, 1)
	if got := request(t, s, "POST", root+"/continue", bad); got.Code != 400 || !strings.Contains(got.Body.String(), "MODEL_NOT_AVAILABLE") {
		t.Fatal("unknown model accepted", got.Code, got.Body.String())
	}
	var next shared.WorkAttempt
	for _, status := range []int{202, 200} {
		got := request(t, s, "POST", root+"/continue", body)
		if got.Code != status || json.Unmarshal(got.Body.Bytes(), &next) != nil {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	if next.Provider != "claude" || next.Model != "second-model" || !next.SelectionRecorded || next.ParentAttemptID != original || next.WorkspaceID != item.WorkspaceID {
		t.Fatal(next)
	}
	if got := request(t, s, "POST", root+"/continue", strings.Replace(body, "second-model", "first-model", 1)); got.Code != 409 {
		t.Fatal("model-changing retry accepted", got.Code)
	}
	repo := orchestration.Repository{DB: s.Store.DB}
	updated, err := repo.Work(context.Background(), project.ID, item.ID)
	if err != nil || updated.Provider != "claude" || updated.Model != "second-model" {
		t.Fatal(updated, err)
	}
	attempts, err := repo.Attempts(context.Background(), project.ID, item.ID)
	if err != nil || len(attempts) != 2 {
		t.Fatal(attempts, err)
	}
	for _, a := range attempts {
		if a.ID == original && (a.Provider != item.Provider || a.Model != item.Model) {
			t.Fatal("rewrote prior model", a)
		}
	}
}
