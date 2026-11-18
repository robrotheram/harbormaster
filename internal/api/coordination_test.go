package api

import (
	"agent-workbench/internal/coordination"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestExchangeRoutesSubmitThroughFactory(t *testing.T) {
	for _, inline := range []bool{false, true} {
		t.Run(fmt.Sprint(inline), func(t *testing.T) { testExchangeWorkRequest(t, inline) })
	}
}
func testExchangeWorkRequest(t *testing.T, inline bool) {
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
	s.RegisterCoordination(mux, service, func(string, string) (coordination.NotePublisher, error) { return nil, coordination.ErrDenied })
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
	if inline {
		body = `{"instruction":"Selected novel request","kind":"research","artifact_paths":["outputs/report.json"]}`
	}
	x, e := service.Publish(c, source.ID, "g", "daily", []byte(body))
	if e != nil {
		t.Fatal(e)
	}
	if w := call("source-token", "/v1/exchange-exports/"+x.ID+"/submit", `{"project_id":"`+dest.ID+`"}`); w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	// Fail the first work-create write while leaving exchange storage available.
	if _, e = s.Store.DB.Exec("ALTER TABLE work_create_requests RENAME TO unavailable_work_create_requests"); e != nil {
		t.Fatal(e)
	}
	if w := call("dest-token", "/v1/exchange-exports/"+x.ID+"/submit", `{"project_id":"`+dest.ID+`"}`); w.Code != 503 || !strings.Contains(w.Body.String(), "STORAGE_UNAVAILABLE") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, tc := range []struct {
		token, project string
		want           int
	}{{"dest-token", dest.ID, 1}, {"source-token", source.ID, 0}} {
		r := httptest.NewRequest("GET", "/v1/exchange-inbox?project_id="+tc.project+"&limit=1", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		var page coordination.InboxPage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != tc.want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	var count int
	if e = s.Store.DB.QueryRow("SELECT count(*) FROM exchange_consumptions").Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
	if _, e = s.Store.DB.Exec("ALTER TABLE unavailable_work_create_requests RENAME TO work_create_requests"); e != nil {
		t.Fatal(e)
	}
	// The factory commits but receipt persistence fails. Retry must recover the
	// same item rather than creating another after this lost acknowledgement.
	if _, e = s.Store.DB.Exec("DROP TABLE exchange_consumptions"); e != nil {
		t.Fatal(e)
	}
	if w := call("dest-token", "/v1/exchange-exports/"+x.ID+"/submit", `{"project_id":"`+dest.ID+`"}`); w.Code != 503 || !strings.Contains(w.Body.String(), "COORDINATION_UNAVAILABLE") {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e = s.Store.DB.Exec(coordination.ConsumptionSchema); e != nil {
		t.Fatal(e)
	}
	var stable string
	for i := 0; i < 2; i++ {
		w := call("dest-token", "/v1/exchange-exports/"+x.ID+"/submit", `{"project_id":"`+dest.ID+`"}`)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var item struct {
			ID string `json:"id"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &item); e != nil {
			t.Fatal(e)
		}
		if i == 0 {
			stable = item.ID
		} else if stable != item.ID {
			t.Fatal("work changed")
		}
	}
	fresh, e := service.Publish(c, source.ID, "g", "next-day", []byte(body))
	if e != nil {
		t.Fatal(e)
	}
	if w := call("dest-token", "/v1/exchange-exports/"+fresh.ID+"/submit", `{"project_id":"`+dest.ID+`"}`); w.Code != 201 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	if e = service.Revoke(c, source.ID, "g"); e != nil {
		t.Fatal(e)
	}
	if w := call("dest-token", "/v1/exchange-exports/"+x.ID+"/submit", `{"project_id":"`+dest.ID+`"}`); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
}
