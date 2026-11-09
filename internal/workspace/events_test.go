package workspace

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestActivityEventsPersistWithoutRawSecrets(t *testing.T) {
	cfg := Config{ID: "ws_logs", Token: "workspace-token", DataDir: t.TempDir()}
	m, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	w := &nativeEvents{m: m, source: "hermes"}
	secret := "DO_NOT_LOG_THIS_CREDENTIAL"
	w.Write([]byte("raw secret " + secret + "\nAWB_EVENT {\"kind\":\"tool.started\",\"tool\":\"terminal\",\"arguments\":\""))
	w.Write([]byte(secret + "\"}\nAWB_EVENT {\"kind\":\"tool.completed\",\"tool\":\"" + secret + "\"}\n"))
	w.Write([]byte(strings.Repeat(secret, 5000) + "\nAWB_EVENT {\"kind\":\"tool.started\",\"tool\":\"memory\"}\n"))
	events := m.events()
	b, _ := json.Marshal(events)
	if len(events) != 3 || strings.Contains(string(b), secret) {
		t.Fatal("raw data logged or valid event lost", string(b))
	}
	m.Close()
	m, e = New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if len(m.events()) != 3 {
		t.Fatal("events lost on reopen")
	}
	for i := 0; i < 1000; i++ {
		m.event("matrix", "message.received", "", "", "")
	}
	var count int
	m.db.QueryRow(`SELECT count(*) FROM activity_events`).Scan(&count)
	if count > 243 {
		t.Fatal("event rate budget exceeded", count)
	}
	m.eventMu.Lock()
	m.eventWindow = time.Now().Add(-2 * time.Minute)
	m.eventMu.Unlock()
	m.event("matrix", "response.started", "", "", "")
}
