package workspace

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func readHistoryTest(t *testing.T, m *Manager, query string) shared.HistoryPage {
	t.Helper()
	w := httptest.NewRecorder()
	m.historyPage(w, httptest.NewRequest("GET", "/v1/tasks?history=1&"+query, nil))
	var page shared.HistoryPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	return page
}
func TestHistoryPrivatePersistenceRedactionPaginationAndBudget(t *testing.T) {
	cfg := Config{ID: "ws_history", DataDir: t.TempDir(), Token: "workspace-exact-secret"}
	m, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	appendRecord := func(id, content string) {
		m.history(shared.HistoryRecord{ID: id, EpisodeID: "ep_test", Source: "hermes", Kind: "tool.result", Content: content})
	}
	appendRecord("hist_first", `{"access_token":"sensitive-token","nested":{"output":"Bearer bearercredential workspace-exact-secret sk-verylongcredentialhere"},"ok":1}`)
	first := readHistoryTest(t, m, "")
	if len(first.Records) != 1 {
		t.Fatal(first)
	}
	r := first.Records[0]
	var payload any
	if !r.Redacted || !json.Valid([]byte(r.Content)) || json.Unmarshal([]byte(r.Content), &payload) != nil {
		t.Fatal("redacted JSON invalid", r)
	}
	for _, secret := range []string{"sensitive-token", "bearercredential", "workspace-exact-secret", "sk-verylongcredentialhere"} {
		if strings.Contains(r.Content, secret) {
			t.Fatal("secret retained")
		}
	}
	appendRecord("hist_large", strings.Repeat("€", 12000))
	second := readHistoryTest(t, m, "")
	if !second.Records[1].Truncated || !utf8.ValidString(second.Records[1].Content) {
		t.Fatal("truncation not explicit/UTF8 safe")
	}
	appendRecord("hist_first", "duplicate")
	if len(readHistoryTest(t, m, "").Records) != 2 {
		t.Fatal("duplicate record")
	}
	page := readHistoryTest(t, m, "limit=1&order=desc")
	if page.Records[0].ID != "hist_large" || page.NextBefore == 0 {
		t.Fatal("newest pagination", page)
	}
	page = readHistoryTest(t, m, "limit=1&before="+strconv.FormatInt(page.NextBefore, 10)+"&order=desc")
	if page.Records[0].ID != "hist_first" {
		t.Fatal("older pagination")
	}
	m.Close()
	m, e = New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if len(readHistoryTest(t, m, "").Records) != 2 {
		t.Fatal("history lost after reopen")
	}
	m.db.Exec(`UPDATE history_state SET bytes_used=? WHERE id=1`, historyBudget)
	appendRecord("hist_first", "duplicate")
	if readHistoryTest(t, m, "").CaptureLimited {
		t.Fatal("duplicate exhausted budget")
	}
	appendRecord("hist_over", "no capacity")
	page = readHistoryTest(t, m, "")
	if !page.CaptureLimited || len(page.Records) != 2 {
		t.Fatal("budget not enforced", page)
	}
}
func TestNativeHistoryOmitsReasoningAndUnauthorisedMatrix(t *testing.T) {
	m, e := New(Config{ID: "ws_history", DataDir: t.TempDir(), Token: "test-token"})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	m.captureHistoryLine("codex", "ep_test", []byte(`{"type":"item.completed","item":{"id":"reason_1","type":"reasoning","text":"PRIVATE_REASONING"}}`))
	m.captureHistoryLine("matrix", "", []byte(`AWB_HISTORY {"episode":"ep_bad","kind":"user.message","content":"FOREIGN_MESSAGE","room_id":"!bad","sender_id":"@bad"}`))
	m.captureHistoryLine("codex", "ep_test", []byte(`{"type":"item.completed","item":{"id":"cmd_1","type":"command_execution","command":"printf proof","aggregated_output":"proof","status":"completed"}}`))
	var transcript t3HistoryTranscript
	if json.Unmarshal([]byte(`{"items":[
 {"visibility":"local","item":{"id":"item:user:message-123","type":"user_message","text":"past user","runId":"run_old"}},
 {"visibility":"local","item":{"id":"item_asst","type":"assistant_message","text":"past reply","runId":"run_old"}},
 {"visibility":"local","item":{"id":"item_reason","type":"reasoning","text":"PRIVATE_REASONING"}},
 {"visibility":"local","item":{"id":"item_secret","type":"secret_request","text":"PRIVATE_SECRET_REQUEST"}},
 {"visibility":"inherited","item":{"id":"item_foreign","type":"user_message","text":"FOREIGN_MESSAGE"}},
 {"visibility":"local","item":{"id":"item_stream","type":"assistant_message","streaming":true,"text":"partial"}}
 ]}`), &transcript) != nil {
		t.Fatal("fixture")
	}
	m.appendT3History(t3HistoryThread{ID: "thread:project:fixture:thread-123"}, transcript)
	m.appendT3History(t3HistoryThread{ID: "thread:project:fixture:thread-123"}, transcript)
	page := readHistoryTest(t, m, "")
	b, _ := json.Marshal(page)
	if len(page.Records) != 4 || strings.Contains(string(b), "PRIVATE_") || strings.Contains(string(b), "FOREIGN_MESSAGE") || strings.Contains(string(b), "partial") {
		t.Fatal("private/foreign content captured or duplicate", string(b))
	}
	if page.Records[2].MessageID != "item:user:message-123" || page.Records[2].ToolCallID != "" || page.Records[2].Model != "" {
		t.Fatal("fabricated metadata")
	}
}

func TestT3HistoryNativeIDBoundary(t *testing.T) {
	for _, id := range []string{"thread:project:123:456", "item:assistant-message:123", "run:123.456"} {
		if !validT3HistoryID.MatchString(id) {
			t.Fatal("valid namespaced native ID rejected", id)
		}
	}
	for _, id := range []string{"", "../secrets", "thread/other", "thread?query=1", "thread#fragment", strings.Repeat("x", 513)} {
		if validT3HistoryID.MatchString(id) {
			t.Fatal("unsafe native ID accepted", id)
		}
	}
}

type historyRoundTrip func(*http.Request) (*http.Response, error)

func (fn historyRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }
func TestT3HistoryCollectsNamespacedIDsWithinWorkspace(t *testing.T) {
	m, e := New(Config{ID: "ws_history", DataDir: t.TempDir(), Token: "test-token"})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	m.historyCollectMu.Lock()
	old := consoleHTTP
	m.console = &console{done: make(chan struct{}), token: "private-test-token", tokenAt: time.Now()}
	calls := 0
	consoleHTTP = &http.Client{Transport: historyRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "127.0.0.1:3773" || r.Header.Get("Authorization") != "Bearer private-test-token" {
			t.Error("incorrect native history boundary")
		}
		var body string
		if r.URL.Path == "/api/orchestration/shell" {
			b, _ := json.Marshal(map[string]any{"projects": []any{map[string]string{"id": "project-current", "workspaceRoot": filepath.Join(m.cfg.DataDir, "files")}, map[string]string{"id": "project-foreign", "workspaceRoot": "/elsewhere"}}, "threads": []any{map[string]string{"id": "thread:project:current:123", "projectId": "project-current", "updatedAt": "now"}, map[string]string{"id": "thread:project:foreign:123", "projectId": "project-foreign", "updatedAt": "now"}}})
			body = string(b)
		} else {
			if r.URL.Path != "/api/orchestration/threads/thread:project:current:123/transcript" {
				t.Error("foreign native project fetched", r.URL.Path)
			}
			calls++
			body = `{"items":[{"visibility":"local","item":{"id":"item:user:123","type":"user_message","text":"local transcript","runId":"run:123"}}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	m.collectT3History(context.Background())
	m.collectT3History(context.Background())
	m.console = nil
	consoleHTTP = old
	m.historyCollectMu.Unlock()
	page := readHistoryTest(t, m, "")
	if calls != 1 || len(page.Records) != 1 || page.Records[0].Content != "local transcript" || page.Records[0].SessionID != "thread:project:current:123" {
		t.Fatal("native import lost or crossed project boundary", calls, page)
	}
}
