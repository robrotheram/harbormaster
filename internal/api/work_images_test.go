package api

import (
	"agent-workbench/internal/shared"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkImageScopeAndLatestVersion(t *testing.T) {
	s, rt := setup(t)
	project := domainProject(t, s, "images")
	foreign := domainProject(t, s, "foreign-images")
	var item shared.WorkItem
	created := request(t, s, "POST", "/v1/work-items", `{"project_id":"`+project.ID+`","instruction":"Draw disk usage","kind":"research","idempotency_key":"image-work"}`)
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &item) != nil {
		t.Fatal(created.Code, created.Body.String())
	}
	root := "/v1/work-items/" + item.ID
	request(t, s, "POST", root+"/run", `{"project_id":"`+project.ID+`","idempotency_key":"image-run"}`)
	item = waitWorkReview(t, s, project.ID, item.ID)
	image := `<svg xmlns="http://www.w3.org/2000/svg"><text>Disk usage</text></svg>`
	if err := os.WriteFile(filepath.Join(rt.base, item.WorkspaceID, "files", "chart.svg"), []byte(image), 0600); err != nil {
		t.Fatal(err)
	}
	body := `{"project_id":"` + project.ID + `","attempt_id":"` + item.LatestAttemptID + `","path":"chart.svg"}`
	got := request(t, s, "POST", root+"/image", body)
	var file shared.FileHandoff
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &file) != nil || string(file.Content) != image || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(got.Code, got.Body.String())
	}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{strings.Replace(body, project.ID, foreign.ID, 1), 404},
		{strings.Replace(body, item.LatestAttemptID, "attempt_old", 1), 404},
		{strings.Replace(body, "chart.svg", "../chart.svg", 1), 400},
		{strings.Replace(body, "chart.svg", ".env/image.svg", 1), 400},
		{strings.Replace(body, "chart.svg", "index.html", 1), 400},
		{strings.Replace(body, "chart.svg", "missing.svg", 1), 404},
	} {
		if r := request(t, s, "POST", root+"/image", tc.body); r.Code != tc.status {
			t.Fatal(tc.body, r.Code, r.Body.String())
		}
	}
}

func TestWorkspaceChartDataAndPresentationContract(t *testing.T) {
	s, rt := setup(t)
	project := domainProject(t, s, "chart-data")
	foreign := domainProject(t, s, "foreign-chart-data")
	body := `{"project_id":"` + project.ID + `","instruction":"Visualize the experiment","presentation":"interactive-v1","kind":"research","idempotency_key":"chart-data"}`
	var item shared.WorkItem
	for _, status := range []int{201, 200} {
		got := request(t, s, "POST", "/v1/work-items", body)
		if got.Code != status || json.Unmarshal(got.Body.Bytes(), &item) != nil {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	instruction, err := os.ReadFile(filepath.Join(rt.base, item.WorkspaceID, "files", item.SpecificationPath))
	if err != nil || !strings.Contains(string(instruction), "workbench-chart") || !strings.HasSuffix(string(instruction), "\n\nCurrent request:\nVisualize the experiment") {
		t.Fatal(string(instruction), err)
	}
	root := "/v1/work-items/" + item.ID
	request(t, s, "POST", root+"/run", `{"project_id":"`+project.ID+`","idempotency_key":"chart-run"}`)
	item = waitWorkReview(t, s, project.ID, item.ID)
	data := `[{"label":"A","value":12},{"label":"B","value":24}]`
	if err := os.WriteFile(filepath.Join(rt.base, item.WorkspaceID, "files", "data.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	payload := `{"project_id":"` + project.ID + `","attempt_id":"` + item.LatestAttemptID + `","path":"data.json"}`
	got := request(t, s, "POST", root+"/data", payload)
	var result shared.FileHandoff
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &result) != nil || string(result.Content) != data {
		t.Fatal(got.Code, got.Body.String())
	}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{strings.Replace(payload, project.ID, foreign.ID, 1), 404},
		{strings.Replace(payload, item.LatestAttemptID, "attempt_old", 1), 404},
		{strings.Replace(payload, "data.json", "../data.json", 1), 400},
		{strings.Replace(payload, "data.json", "data.html", 1), 400},
		{strings.Replace(payload, "data.json", ".env/data.json", 1), 400},
	} {
		if response := request(t, s, "POST", root+"/data", tc.body); response.Code != tc.status {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	followup := `{"project_id":"` + project.ID + `","expected_attempt_id":"` + item.LatestAttemptID + `","instruction":"Compare the values","presentation":"interactive-v1","idempotency_key":"chart-followup"}`
	var next shared.WorkAttempt
	got = request(t, s, "POST", root+"/continue", followup)
	if got.Code != 202 || json.Unmarshal(got.Body.Bytes(), &next) != nil {
		t.Fatal(got.Code, got.Body.String())
	}
	instruction, err = os.ReadFile(filepath.Join(rt.base, item.WorkspaceID, "files", next.SpecificationPath))
	if err != nil || !strings.Contains(string(instruction), "workbench-chart") || !strings.HasSuffix(string(instruction), "\n\nCurrent request:\nCompare the values") {
		t.Fatal(string(instruction), err)
	}
}
