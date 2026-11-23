package api

import (
	"agent-workbench/internal/dashboard"
	"agent-workbench/internal/shared"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// Exercise the actual browser ingress and API together: mocking /ui-api would
// miss an endpoint present in the API but absent from the dashboard allowlist.
func TestOverviewThroughDashboardIngress(t *testing.T) {
	s, _ := setup(t)
	upstream := httptest.NewServer(s.Handler())
	defer upstream.Close()
	ingress, err := dashboard.NewInteractiveHandler(upstream.URL, upstream.URL, "control-token")
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://localhost/ui-api"+path, strings.NewReader(body))
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Workbench-Request", "1")
		w := httptest.NewRecorder()
		ingress.ServeHTTP(w, r)
		return w
	}
	p := domainProject(t, s, "real-ingress")
	res := call("GET", "/projects/"+p.ID+"/capabilities", "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"can_start":true`) {
		t.Fatal("capabilities blocked", res.Code, res.Body)
	}
	// A legitimate description can exceed the old generic 4 KiB proxy limit.
	description := strings.Repeat("Acceptance criteria and context.\n", 220)
	data, _ := json.Marshal(map[string]any{"project_id": p.ID, "title": "Verify the real Overview flow", "description": description, "kind": "software", "idempotency_key": "ui-planned"})
	res = call("POST", "/board", string(data))
	var card boardCard
	if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &card) != nil {
		t.Fatal("create blocked", res.Code, res.Body)
	}
	worker := domainWorkspace(t, s, "control-token", p.ID, "ingress-worker")
	body := fmt.Sprintf(`{"project_id":%q,"revision":%d,"workspace_id":%q}`, p.ID, card.Revision, worker.ID)
	var id string
	for i := 0; i < 2; i++ {
		res = call("POST", "/board/"+card.ID+"/start", body)
		var a shared.WorkAttempt
		if (res.Code != 202 && res.Code != 200) || json.Unmarshal(res.Body.Bytes(), &a) != nil {
			t.Fatal("start blocked", res.Code, res.Body)
		}
		if a.WorkItemID != card.ID || (i == 1 && id != a.ID) {
			t.Fatal("start identity/retry mismatch")
		}
		id = a.ID
	}
	res = call("GET", "/work-items/"+card.ID+"?project_id="+p.ID, "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), id) {
		t.Fatal("cannot resume started task", res.Code, res.Body)
	}
}
