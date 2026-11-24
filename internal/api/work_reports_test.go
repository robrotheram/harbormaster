package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"agent-workbench/internal/shared"
)

func TestSaveWorkReportUsesOnlyTheTaskWorker(t *testing.T) {
	s, _ := setup(t)
	project := domainProject(t, s, "work-reports")
	worker := domainWorkspace(t, s, "control-token", project.ID, "reports")
	other := domainWorkspace(t, s, "control-token", project.ID, "different-worker")
	created := request(t, s, "POST", "/v1/work-items", `{"project_id":"`+project.ID+`","workspace_id":"`+worker.ID+`","instruction":"Study the observations.","kind":"data-science","idempotency_key":"report-task"}`)
	var item shared.WorkItem
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &item) != nil {
		t.Fatal(created.Code, created.Body.String())
	}
	endpoint := "/v1/work-items/" + item.ID
	save := func(workerID, path, markdown string, previous *string, want int) {
		t.Helper()
		body, _ := json.Marshal(struct {
			ProjectID string `json:"project_id"`
			WorkerID  string `json:"worker_id"`
			shared.ReportWrite
		}{project.ID, workerID, shared.ReportWrite{Path: path, Markdown: markdown, Previous: previous}})
		got := request(t, s, "POST", endpoint+"/reports", string(body))
		if got.Code != want {
			t.Fatalf("save %q: got %d, want %d (%s)", path, got.Code, want, got.Body.String())
		}
	}
	initial := "# First report\n\nInitial findings."
	updated := "# First report\n\nRevised findings."
	const path = "reports/field-report.md"
	save(other.ID, path, initial, nil, http.StatusNotFound)
	save(worker.ID, "work-requests/private-report.md", initial, &initial, http.StatusBadRequest)
	save(worker.ID, "reports/../field-report.md", initial, nil, http.StatusBadRequest)
	save(worker.ID, path, initial, nil, http.StatusOK)
	save(worker.ID, path, initial, nil, http.StatusOK)
	save(worker.ID, path, updated, nil, http.StatusConflict)
	save(worker.ID, path, updated, &initial, http.StatusOK)
	save(worker.ID, path, "# Outdated edit", &initial, http.StatusConflict)
	save(worker.ID, "reports/second.md", "# Second report", nil, http.StatusOK)
	loaded := request(t, s, "POST", endpoint+"/files?project_id="+project.ID, `{"path":"`+path+`"}`)
	var file shared.FileHandoff
	if loaded.Code != http.StatusOK || json.Unmarshal(loaded.Body.Bytes(), &file) != nil || string(file.Content) != updated {
		t.Fatal(loaded.Code, loaded.Body.String())
	}
}
