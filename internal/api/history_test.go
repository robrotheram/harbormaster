package api

import (
	"agent-workbench/internal/shared"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// A snapshot source that gains a record after the first page.
type historyFixtureRuntime struct {
	*embeddedRuntime
	reads    int
	failPage bool
}

func (rt *historyFixtureRuntime) Agent(ctx context.Context, id, method, path string, b []byte) (int, []byte, error) {
	u, _ := url.Parse(path)
	if method != "GET" || u.Query().Get("history") != "1" {
		return rt.embeddedRuntime.Agent(ctx, id, method, path, b)
	}
	rt.reads++
	after, _ := strconv.Atoi(u.Query().Get("after"))
	until, _ := strconv.Atoi(u.Query().Get("until"))
	if until == 0 {
		until = 25
		if rt.reads > 1 {
			until = 26
		}
	}
	if rt.failPage && after > 0 {
		return 503, nil, nil
	}
	page := shared.HistoryPage{SchemaVersion: 1, HighWatermark: 25, Records: []shared.HistoryRecord{}}
	for i := after + 1; i <= until && len(page.Records) < 20; i++ {
		page.Records = append(page.Records, shared.HistoryRecord{ID: fmt.Sprintf("hist_%d", i), Seq: int64(i), EpisodeID: "ep_review", Source: "hermes", Kind: "assistant.message", Content: "PRIVATE_HISTORY_CONTENT", Purpose: "test", SchemaVersion: 1})
	}
	if len(page.Records) == 20 {
		page.NextAfter = page.Records[19].Seq
	}
	out, _ := json.Marshal(page)
	return 200, out, nil
}
func TestHistorySnapshotExportFeedbackAndScope(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "history")
	before := rt.starts
	if w := request(t, s, "GET", "/v1/workspaces/"+ws.ID+"/history", ""); w.Code != 409 || rt.starts != before {
		t.Fatal("read woke stopped workspace", w.Code)
	}
	if w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"start-history"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	fixture := &historyFixtureRuntime{embeddedRuntime: rt}
	s.Runtime = fixture
	s.cfg.ProjectTokens = map[string]string{"project-token": "default", "foreign-token": "foreign"}
	for _, path := range []string{"/history", "/history/export"} {
		if w := domainRequest(t, s, "foreign-token", "GET", "/v1/workspaces/"+ws.ID+path, ""); w.Code != 404 {
			t.Fatal("foreign history exposed", w.Code)
		}
	}
	if w := domainRequest(t, s, "project-token", "POST", "/v1/workspaces/"+ws.ID+"/history/feedback", `{"episode_id":"ep_review","rating":1}`); w.Code != 403 {
		t.Fatal("non-operator supplied reward", w.Code)
	}
	for _, body := range []string{`{"episode_id":"ep_review"}`, `{"episode_id":"ep_review","rating":2}`} {
		if w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/history/feedback", body); w.Code != 400 {
			t.Fatal("invalid reward accepted", w.Code)
		}
	}
	if w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/history/feedback", `{"episode_id":"ep_review","rating":1}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	fixture.reads = 0
	w := request(t, s, "GET", "/v1/workspaces/"+ws.ID+"/history/export", "")
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal(w.Code, w.Header())
	}
	lines := bytes.Split(bytes.TrimSpace(w.Body.Bytes()), []byte("\n"))
	if len(lines) != 27 {
		t.Fatal("not complete snapshot", len(lines))
	}
	digest := sha256.New()
	for _, line := range lines[1 : len(lines)-1] {
		var item struct {
			Record   shared.HistoryRecord `json:"record"`
			Feedback struct {
				Rating   int    `json:"rating"`
				Reviewer string `json:"reviewer"`
			} `json:"operator_feedback"`
		}
		json.Unmarshal(line, &item)
		if item.Record.Purpose != "" || item.Record.Seq > 25 || item.Feedback.Rating != 1 || item.Feedback.Reviewer != "local-operator" {
			t.Fatal("snapshot or label provenance wrong")
		}
		digest.Write(append(line, '\n'))
	}
	var footer struct {
		Type    string `json:"type"`
		Records int    `json:"records"`
		SHA     string `json:"sha256"`
	}
	json.Unmarshal(lines[len(lines)-1], &footer)
	if footer.Type != "export_complete" || footer.Records != 25 || footer.SHA != hex.EncodeToString(digest.Sum(nil)) {
		t.Fatal("export integrity", footer)
	}
	// Content is only streamed; metadata and ratings contain no transcript text.
	rows, e := s.Store.DB.Query(`SELECT metadata FROM workspaces UNION ALL SELECT metadata FROM task_metadata UNION ALL SELECT episode_id FROM history_feedback`)
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var v string
		rows.Scan(&v)
		if strings.Contains(v, "PRIVATE_HISTORY_CONTENT") {
			t.Fatal("content stored in control plane")
		}
	}
	rows.Close()
	fixture.reads = 0
	fixture.failPage = true
	w = request(t, s, "GET", "/v1/workspaces/"+ws.ID+"/history/export", "")
	if !strings.Contains(w.Body.String(), `"type":"export_error"`) || strings.Contains(w.Body.String(), `"type":"export_complete"`) {
		t.Fatal("partial export declared complete")
	}
}

func TestHistoryPurposeUsesOperatorMetadataAndSurvivesRestart(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "history-purpose")
	if w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"start-purpose"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	fixture := &historyFixtureRuntime{embeddedRuntime: rt}
	s.Runtime = fixture
	path := "/v1/workspaces/" + ws.ID + "/history"
	s.cfg.ProjectTokens = map[string]string{"project-token": "default"}
	if w := domainRequest(t, s, "project-token", "POST", path+"/purpose", `{"episode_id":"ep_review","purpose":"test"}`); w.Code != 403 {
		t.Fatal("project/agent credential may label operator runs", w.Code)
	}
	read := func(server *Server) shared.HistoryPage {
		w := request(t, server, "GET", path, "")
		var out struct {
			Page shared.HistoryPage `json:"page"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal(w.Code, w.Body)
		}
		return out.Page
	}
	for _, v := range read(s).Records {
		if v.Purpose != "" {
			t.Fatal("guest forged a run classification")
		}
	}
	for _, body := range []string{`{"episode_id":"ep_review","purpose":"invalid"}`, `{"episode_id":"ep_review"}`} {
		if w := request(t, s, "POST", path+"/purpose", body); w.Code != 400 {
			t.Fatal("invalid purpose accepted", w.Code)
		}
	}
	if w := request(t, s, "POST", path+"/purpose", `{"episode_id":"ep_missing","purpose":"test"}`); w.Code != 404 {
		t.Fatal("missing episode was annotated", w.Code)
	}
	if w := request(t, s, "POST", path+"/purpose", `{"episode_id":"ep_review","purpose":"test"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	restarted := New(s.Store, s.Runtime, s.cfg)
	for _, v := range read(restarted).Records {
		if v.Purpose != "test" || v.Content != "PRIVATE_HISTORY_CONTENT" {
			t.Fatal("run purpose lost or transcript altered", v.Purpose)
		}
	}
	other := create(t, s, "other-purpose")
	purposes, err := s.Store.HistoryPurposes(context.Background(), other.ID)
	if err != nil || len(purposes) != 0 {
		t.Fatal("labels crossed workspace boundary", purposes, err)
	}
	fixture.reads = 0
	w := request(t, restarted, "GET", path+"/export", "")
	for _, line := range bytes.Split(bytes.TrimSpace(w.Body.Bytes()), []byte("\n")) {
		var item struct {
			Type   string               `json:"type"`
			Record shared.HistoryRecord `json:"record"`
		}
		json.Unmarshal(line, &item)
		if item.Type == "record" && item.Record.Purpose != "test" {
			t.Fatal("purpose absent from training export")
		}
	}
	if w := request(t, restarted, "POST", path+"/purpose", `{"episode_id":"ep_review","purpose":"normal"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	for _, v := range read(restarted).Records {
		if v.Purpose != "normal" {
			t.Fatal("purpose correction not applied")
		}
	}
}
