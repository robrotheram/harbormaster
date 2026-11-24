package api

import (
	"agent-workbench/internal/dashboard"
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWorkHistoryStreamsThroughDashboardAndResumes(t *testing.T) {
	s, rt := setup(t)
	release := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	inference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"streamed reply"}}]}`)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(inference.Close)
	t.Cleanup(finish)
	rt.upstream = inference.URL
	project := domainProject(t, s, "stream-project")
	foreign := domainProject(t, s, "stream-foreign")
	var item shared.WorkItem
	res := request(t, s, "POST", "/v1/work-items", fmt.Sprintf(`{"project_id":%q,"instruction":"streamed request","kind":"research","idempotency_key":"stream-create"}`, project.ID))
	if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &item) != nil {
		t.Fatal(res.Code, res.Body.String())
	}
	var accepted shared.WorkAttempt
	res = request(t, s, "POST", "/v1/work-items/"+item.ID+"/run", fmt.Sprintf(`{"project_id":%q,"idempotency_key":"stream-run"}`, project.ID))
	if res.Code != 202 || json.Unmarshal(res.Body.Bytes(), &accepted) != nil {
		t.Fatal(res.Code, res.Body.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.tickOrchestration(ctx)
	repo := orchestration.Repository{DB: s.Store.DB}
	attempt, err := repo.Attempt(ctx, project.ID, item.ID, accepted.ID)
	if err != nil || attempt.TaskID == "" {
		t.Fatalf("attempt not dispatched: %v", err)
	}

	control := httptest.NewServer(s.Handler())
	defer control.Close()
	ingress, err := dashboard.NewInteractiveHandler(control.URL, control.URL, "control-token")
	if err != nil {
		t.Fatal(err)
	}
	ui := httptest.NewServer(ingress)
	defer ui.Close()
	path := ui.URL + "/ui-api/work-items/" + item.ID + "/attempts/" + attempt.ID + "/history/events?after=0&project_id="
	open := func(project, lastID string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, "GET", path+project, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Last-Event-ID", lastID)
		got, err := ui.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { got.Body.Close() })
		return got
	}
	if denied := open(foreign.ID, ""); denied.StatusCode != 404 {
		t.Fatal("foreign project received stream", denied.StatusCode)
	}
	if invalid := open(project.ID, "-1"); invalid.StatusCode != 400 {
		t.Fatal("invalid cursor accepted", invalid.StatusCode)
	}
	first := open(project.ID, "")
	defer first.Body.Close()
	if first.StatusCode != 200 || first.Header.Get("Content-Type") != "text/event-stream" || !strings.Contains(first.Header.Get("Cache-Control"), "no-store") {
		t.Fatal("invalid stream response", first.StatusCode, first.Header)
	}
	reader := bufio.NewScanner(first.Body)
	var last int64
	for last == 0 {
		page := scanWorkHistory(t, reader)
		for _, record := range page.Records {
			if record.Kind == "run.started" {
				last = record.Seq
			}
		}
	}
	first.Body.Close()
	reconnected := open(project.ID, strconv.FormatInt(last, 10))
	defer reconnected.Body.Close()
	finish()
	reader = bufio.NewScanner(reconnected.Body)
	for {
		page := scanWorkHistory(t, reader)
		for _, record := range page.Records {
			if record.Seq <= last || record.EpisodeID != attempt.TaskID {
				t.Fatal("stream replayed earlier or foreign history", record.Seq)
			}
			if record.Kind == "assistant.message" {
				if record.Content != "streamed reply" {
					t.Fatal("incorrect streamed reply")
				}
				return
			}
		}
	}
}

func scanWorkHistory(t *testing.T, scanner *bufio.Scanner) shared.HistoryPage {
	t.Helper()
	for scanner.Scan() {
		if payload, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			var value struct{ Page shared.HistoryPage }
			if err := json.Unmarshal([]byte(payload), &value); err != nil || value.Page.SchemaVersion != 1 {
				t.Fatalf("invalid history frame: %s", payload)
			}
			return value.Page
		}
	}
	t.Fatal("stream closed before history arrived", scanner.Err())
	return shared.HistoryPage{}
}
