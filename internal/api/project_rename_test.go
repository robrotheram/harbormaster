package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectRenamePreservesIdentityAndRequiresOperator(t *testing.T) {
	s, _ := setup(t)
	p := domainProject(t, s, "Original")
	p.DeploymentNamespace = "project-namespace"
	p.NamespaceReady = true
	body, _ := json.Marshal(p)
	if _, err := s.Store.DB.Exec("UPDATE projects SET metadata=$1 WHERE id=$2", string(body), p.ID); err != nil {
		t.Fatal(err)
	}
	path := "/v1/projects/" + p.ID + "/rename"
	s.cfg.ProjectTokens = map[string]string{"agent": p.ID}
	if w := domainRequest(t, s, "agent", "POST", path, `{"name":"Denied"}`); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, name := range []string{"", "   ", "line\nbreak", strings.Repeat("x", 81)} {
		b, _ := json.Marshal(map[string]string{"name": name})
		if w := request(t, s, "POST", path, string(b)); w.Code != 400 {
			t.Fatal(name, w.Code)
		}
	}
	for i := 0; i < 2; i++ {
		w := request(t, s, "POST", path, `{"name":"  Renamed  "}`)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	stored, err := s.Store.Project(context.Background(), p.ID)
	p.Name = "Renamed"
	if err != nil || stored != p {
		t.Fatal("rename changed project identity or namespace", stored, err)
	}
	if w := request(t, s, "POST", "/v1/projects/missing/rename", `{"name":"Missing"}`); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if _, err := s.Store.DB.Exec(`INSERT INTO deleted_resources(kind,id) VALUES('project',$1)`, p.ID); err != nil {
		t.Fatal(err)
	}
	if w := request(t, s, "POST", path, `{"name":"Revive"}`); w.Code != 404 {
		t.Fatal("renamed deleted project", w.Code)
	}
}
