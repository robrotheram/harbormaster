package api

import (
	"agent-workbench/internal/coordination"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoordinationCompositionKeepsBodiesInOwningWorkspace(t *testing.T) {
	s, rt := setup(t)
	source, dest := domainProject(t, s, "personal-coordination"), domainProject(t, s, "research-coordination")
	s.cfg.ProjectTokens = map[string]string{"personal": source.ID, "research": dest.ID}
	sourceWS := domainWorkspace(t, s, "personal", source.ID, "personal-store")
	destWS := domainWorkspace(t, s, "research", dest.ID, "research-store")
	for _, binding := range []struct{ project, workspace string }{{source.ID, sourceWS.ID}, {dest.ID, destWS.ID}} {
		body := `{"workspace_id":"` + binding.workspace + `","notes_enabled":true}`
		if got := request(t, s, "POST", "/v1/projects/"+binding.project+"/coordination", body); got.Code != 200 {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	grant := `{"id":"personal-summary","source":"` + source.ID + `","destination":"` + dest.ID + `","operation":"summary"}`
	if got := request(t, s, "POST", "/v1/project-exchanges", grant); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	if got := request(t, s, "POST", "/v1/project-exchanges/personal-summary/approve", `{"project_id":"`+dest.ID+`"}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	published := domainRequest(t, s, "personal", "POST", "/v1/project-exchanges/personal-summary/exports", `{"project_id":"`+source.ID+`","occurrence":"daily-one","body":"selected-summary-only"}`)
	if published.Code != 200 {
		t.Fatal(published.Code, published.Body.String())
	}
	var x coordination.Export
	if err := json.Unmarshal(published.Body.Bytes(), &x); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(rt.base, sourceWS.ID, "files", "coordination", "exports", x.Reference))
	if err != nil || string(body) != "selected-summary-only" {
		t.Fatal("missing project-owned export", err)
	}
	read := domainRequest(t, s, "research", "GET", "/v1/exchange-exports/"+x.ID, "")
	if read.Code != 200 || !strings.Contains(read.Body.String(), "selected-summary-only") {
		t.Fatal(read.Code, read.Body.String())
	}
	note := `{"project_id":"` + dest.ID + `","workspace_id":"` + destWS.ID + `","write":{"path":"daily.md","body":"Read selected summary","expected_digest":"","occurrence":"daily-one","sources":["https://example.com/paper"]}}`
	for i := 0; i < 2; i++ {
		if got := domainRequest(t, s, "research", "POST", "/v1/note-publications", note); got.Code != 200 {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	if got := domainRequest(t, s, "personal", "POST", "/v1/note-publications", note); got.Code != 404 {
		t.Fatal("foreign note write", got.Code)
	}
	if got := request(t, s, "POST", "/v1/project-exchanges/personal-summary/revoke", `{"project_id":"`+source.ID+`"}`); got.Code != 200 {
		t.Fatal(got.Code)
	}
	if got := domainRequest(t, s, "research", "GET", "/v1/exchange-exports/"+x.ID, ""); got.Code != 403 {
		t.Fatal("revoked export exposed", got.Code)
	}
}
