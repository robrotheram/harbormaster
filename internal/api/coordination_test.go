package api

import (
	"agent-workbench/internal/coordination"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestExchangeRoutesSubmitThroughFactory(t *testing.T) {
	s, _ := setup(t)
	source, dest := domainProject(t, s, "exchange-source"), domainProject(t, s, "exchange-destination")
	s.cfg.ProjectTokens = map[string]string{"source-token": source.ID, "dest-token": dest.ID}
	ws := domainWorkspace(t, s, "dest-token", dest.ID, "target")
	if _, e := s.Store.DB.Exec(coordination.Schema); e != nil {
		t.Fatal(e)
	}
	root, e := os.OpenRoot(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer root.Close()
	service := &coordination.Service{DB: s.Store.DB, Blobs: func(p string) (coordination.BlobStore, error) {
		if p != source.ID {
			return nil, coordination.ErrDenied
		}
		return coordination.Files{Root: root}, nil
	}}
	mux := http.NewServeMux()
	s.registerWork(mux)
	s.RegisterCoordination(mux, service, func(string, string) (coordination.Notes, error) { return coordination.Notes{}, coordination.ErrDenied })
	handler := s.authenticate(mux)
	call := func(token, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := call("source-token", "/v1/project-exchanges", `{"id":"g","source":"`+source.ID+`","destination":"`+dest.ID+`","operation":"work-request"}`); w.Code != 403 {
		t.Fatal(w.Code)
	}
	c := context.Background()
	if e = service.Create(c, source.ID, coordination.Grant{ID: "g", Source: source.ID, Destination: dest.ID, Operation: "work-request"}); e != nil {
		t.Fatal(e)
	}
	if e = service.Approve(c, dest.ID, "g"); e != nil {
		t.Fatal(e)
	}
	body := `{"workspace_id":"` + ws.ID + `","specification_path":"specs/daily.json","specification_digest":"` + strings.Repeat("a", 64) + `","kind":"research"}`
	x, e := service.Publish(c, source.ID, "g", "daily", []byte(body))
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		w := call("dest-token", "/v1/exchange-exports/"+x.ID+"/submit", `{"project_id":"`+dest.ID+`"}`)
		if (i == 0 && w.Code != 201) || (i == 1 && w.Code != 200) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if e = service.Revoke(c, source.ID, "g"); e != nil {
		t.Fatal(e)
	}
	if w := call("dest-token", "/v1/exchange-exports/"+x.ID+"/submit", `{"project_id":"`+dest.ID+`"}`); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
}
