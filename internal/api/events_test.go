package api

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLogsPersistAndStayProjectScoped(t *testing.T) {
	s, _ := setup(t)
	a, b := domainProject(t, s, "logs-a"), domainProject(t, s, "logs-b")
	makeWS := func(project, key string) shared.Workspace {
		w := request(t, s, "POST", "/v1/workspaces", `{"project_id":"`+project+`","idempotency_key":"`+key+`"}`)
		var ws shared.Workspace
		if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &ws) != nil {
			t.Fatal(w.Code)
		}
		return ws
	}
	wa, wb := makeWS(a.ID, "logs-a"), makeWS(b.ID, "logs-b")
	good := shared.ActivityEvent{ID: shared.NewID("evt_"), At: time.Now().UTC(), Source: "hermes", Kind: "tool.started", Tool: "terminal"}
	bad := good
	bad.ID = shared.NewID("evt_")
	bad.Tool = "PRIVATE_CREDENTIAL_SENTINEL"
	body, _ := json.Marshal(map[string]any{"events": []shared.ActivityEvent{good, bad}, "events_supported": true})
	s.captureEvents(context.Background(), wa, body)
	s.captureEvents(context.Background(), wa, body)
	s.captureEvents(context.Background(), wb, body)
	s.cfg.ProjectTokens = map[string]string{"project-a": a.ID}
	for _, path := range []string{"/v1/workspace-logs?project_id=" + b.ID, "/v1/workspaces/" + wb.ID + "/logs"} {
		w := domainRequest(t, s, "project-a", "GET", path, "")
		if w.Code != 404 {
			t.Fatal("foreign logs exposed", w.Code)
		}
	}
	w := domainRequest(t, s, "project-a", "GET", "/v1/workspace-logs?project_id="+a.ID, "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "PRIVATE_CREDENTIAL_SENTINEL") || strings.Contains(w.Body.String(), wb.ID) {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		Items []struct {
			Kind string `json:"kind"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	count := 0
	for _, v := range out.Items {
		if v.Kind == "tool.started" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("duplicate ingestion", count)
	}
	restarted := New(s.Store, s.Runtime, s.cfg)
	w = domainRequest(t, restarted, "project-a", "GET", "/v1/workspaces/"+wa.ID+"/logs", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "tool.started") {
		t.Fatal("stopped workspace logs lost", w.Code)
	}
}
