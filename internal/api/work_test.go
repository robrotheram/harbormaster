package api

import (
	"agent-workbench/internal/shared"
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkDraftOwnershipIdempotencyAndCancellation(t *testing.T) {
	s, _ := setup(t)
	a, b := domainProject(t, s, "work-a"), domainProject(t, s, "work-b")
	s.cfg.ProjectTokens = map[string]string{"a": a.ID, "b": b.ID}
	ws := domainWorkspace(t, s, "a", a.ID, "workspace")
	body := `{"project_id":"` + a.ID + `","workspace_id":"` + ws.ID + `","specification_path":"specs/task.json","specification_digest":"` + strings.Repeat("a", 64) + `","kind":"software","idempotency_key":"work"}`
	response := domainRequest(t, s, "a", "POST", "/v1/work-items", body)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	var item shared.WorkItem
	if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.State != "draft" {
		t.Fatal("unverified work accepted for execution")
	}
	if got := domainRequest(t, s, "a", "POST", "/v1/work-items", body); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	if got := domainRequest(t, s, "a", "POST", "/v1/work-items", strings.Replace(body, "task.json", "changed.json", 1)); got.Code != 409 {
		t.Fatal("conflicting retry", got.Code)
	}
	if got := domainRequest(t, s, "a", "POST", "/v1/work-items", strings.Replace(body, "specs/task.json", "../task.json", 1)); got.Code != 400 {
		t.Fatal("traversal", got.Code)
	}
	for _, path := range []string{"/v1/work-items?project_id=" + a.ID, "/v1/work-items/" + item.ID + "?project_id=" + a.ID} {
		if got := domainRequest(t, s, "b", "GET", path, ""); got.Code != 404 {
			t.Fatal("foreign read", got.Code)
		}
	}
	if got := domainRequest(t, s, "b", "GET", "/v1/work-items/"+item.ID, ""); got.Code != 404 {
		t.Fatal("foreign item", got.Code)
	}
	if got := domainRequest(t, s, "b", "POST", "/v1/work-items/"+item.ID+"/cancel", `{"project_id":"`+b.ID+`"}`); got.Code != 404 {
		t.Fatal("foreign cancellation", got.Code)
	}
	for i := 0; i < 2; i++ {
		got := domainRequest(t, s, "a", "POST", "/v1/work-items/"+item.ID+"/cancel", `{"project_id":"`+a.ID+`"}`)
		if got.Code != 200 || !strings.Contains(got.Body.String(), `"state":"cancelled"`) {
			t.Fatal(got.Code, got.Body.String())
		}
	}
}

func TestWorkExecutionRoutesProjectFencing(t *testing.T) {
	s, _ := setup(t)
	a, b := domainProject(t, s, "execution-a"), domainProject(t, s, "execution-b")
	s.cfg.ProjectTokens = map[string]string{"a": a.ID, "b": b.ID}
	ws := domainWorkspace(t, s, "a", a.ID, "workspace")
	body := `{"project_id":"` + a.ID + `","workspace_id":"` + ws.ID + `","specification_path":"work.json","specification_digest":"` + strings.Repeat("a", 64) + `","kind":"research","idempotency_key":"create"}`
	response := domainRequest(t, s, "a", "POST", "/v1/work-items", body)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	var item shared.WorkItem
	json.Unmarshal(response.Body.Bytes(), &item)
	runBody := `{"project_id":"` + a.ID + `","idempotency_key":"run"}`
	var attempt shared.WorkAttempt
	for i := 0; i < 2; i++ {
		response = domainRequest(t, s, "a", "POST", "/v1/work-items/"+item.ID+"/run", runBody)
		if response.Code != 202 {
			t.Fatal(response.Code, response.Body.String())
		}
		var got shared.WorkAttempt
		json.Unmarshal(response.Body.Bytes(), &got)
		if i > 0 && got.ID != attempt.ID {
			t.Fatal("duplicate execution")
		}
		attempt = got
	}
	response = domainRequest(t, s, "a", "GET", "/v1/work-items/"+item.ID+"/attempts", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), attempt.ID) {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, suffix := range []string{"/attempts", "/artifacts/private/content"} {
		response = domainRequest(t, s, "b", "GET", "/v1/work-items/"+item.ID+suffix, "")
		if response.Code != 404 {
			t.Fatal("foreign execution exposed", response.Code)
		}
	}
	response = domainRequest(t, s, "a", "POST", "/v1/work-items/"+item.ID+"/review", `{"project_id":"`+a.ID+`","accepted":true}`)
	if response.Code != 409 {
		t.Fatal("queued work accepted", response.Code)
	}
	response = domainRequest(t, s, "a", "POST", "/v1/work-items/"+item.ID+"/cancel", `{"project_id":"`+a.ID+`"}`)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"state":"running"`) {
		t.Fatal(response.Code, response.Body.String())
	}
}
