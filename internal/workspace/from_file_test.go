package workspace

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agent-workbench/internal/shared"
)

func TestFileAcceptanceRecoversBeforeReadingEditedOrDeletedSource(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"finished"}}]}`)
	}))
	defer upstream.Close()
	cfg := Config{ID: "ws_test", ProjectID: "proj_one", HomeID: "home_one", DataDir: t.TempDir(), Token: "test-workspace-token", InferenceURL: upstream.URL, Model: "fixture"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(cfg.DataDir, "files/task.md")
	if err = os.WriteFile(file, []byte("Original instruction"), 0600); err != nil {
		t.Fatal(err)
	}
	body := `{"instruction_path":"task.md","idempotency_key":"occurrence-one","provider":""}`
	first := invoke(t, m.Handler(), "POST", "/v1/tasks/from-file", body)
	if first.Code != 202 {
		t.Fatalf("accept: %d %s", first.Code, first.Body)
	}
	var task shared.Task
	json.Unmarshal(first.Body.Bytes(), &task)
	waitTask(t, m, task.ID)
	if err = os.WriteFile(file, []byte("Changed instruction with different side effects"), 0600); err != nil {
		t.Fatal(err)
	}
	retry := invoke(t, m.Handler(), "POST", "/v1/tasks/from-file", body)
	var again shared.Task
	json.Unmarshal(retry.Body.Bytes(), &again)
	if retry.Code != 200 || again.ID != task.ID {
		t.Fatal("lost acknowledgement replayed changed content")
	}
	m.Close()
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	m, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	retry = invoke(t, m.Handler(), "POST", "/v1/tasks/from-file", body)
	json.Unmarshal(retry.Body.Bytes(), &again)
	if retry.Code != 200 || again.ID != task.ID {
		t.Fatal("restart required original file or duplicated execution")
	}
	conflict := invoke(t, m.Handler(), "POST", "/v1/tasks/from-file", `{"instruction_path":"other.md","idempotency_key":"occurrence-one"}`)
	if conflict.Code != 409 {
		t.Fatal("file identity changed under an accepted key")
	}
	if _, err = os.Stat(filepath.Join(cfg.DataDir, "homes/home_one/workspace.db")); err != nil {
		t.Fatal("private home state missing")
	}
	for _, path := range []string{"../workspace.db", ".env", "/etc/passwd"} {
		b, _ := json.Marshal(map[string]string{"instruction_path": path, "idempotency_key": "escape-key"})
		if invoke(t, m.Handler(), "POST", "/v1/tasks/from-file", string(b)).Code != 400 {
			t.Fatal("unconfined source accepted")
		}
	}
}

func TestHomeOwnershipAndSingleWriterFence(t *testing.T) {
	cfg := Config{ID: "ws_test", ProjectID: "proj_one", HomeID: "home_one", DataDir: t.TempDir(), Token: "test-workspace-token"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := New(cfg); err == nil {
		other.Close()
		t.Fatal("second supervisor acquired a writable home")
	}
	m.Close()
	for _, change := range []func(*Config){func(c *Config) { c.ProjectID = "proj_two" }, func(c *Config) { c.HomeID = "home_two" }, func(c *Config) { c.ID = "ws_other" }} {
		other := cfg
		change(&other)
		if m, err := New(other); err == nil {
			m.Close()
			t.Fatal("persistent content reassigned")
		}
	}
}

func TestFileReceiptRecoveryPrecedesBusyAdmission(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"finished"}}]}`)
	}))
	defer upstream.Close()
	cfg := Config{ID: "ws_busy_receipt", DataDir: t.TempDir(), Token: "test-workspace-token", InferenceURL: upstream.URL, Model: "fixture"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = os.WriteFile(filepath.Join(cfg.DataDir, "files", "work.md"), []byte("Describe workspace"), 0600); err != nil {
		t.Fatal(err)
	}
	body := `{"instruction_path":"work.md","idempotency_key":"accepted-one"}`
	first := invoke(t, m.Handler(), "POST", "/v1/tasks/from-file", body)
	if first.Code != 202 {
		t.Fatal(first.Code, first.Body.String())
	}
	var task shared.Task
	json.Unmarshal(first.Body.Bytes(), &task)
	waitTask(t, m, task.ID)
	if _, err = m.db.Exec(`INSERT INTO recipe_jobs(id,idem_key,template,state,created_at,timeout_seconds) VALUES('busy_job','busy','research-v1','running','2026-11-10T00:00:00Z',600)`); err != nil {
		t.Fatal(err)
	}
	if got := invoke(t, m.Handler(), "POST", "/v1/tasks/from-file", body); got.Code != 200 {
		t.Fatal("busy recovery lost its receipt", got.Code)
	}
	if got := invoke(t, m.Handler(), "POST", "/v1/tasks/from-file", `{"instruction_path":"work.md","idempotency_key":"new-one"}`); got.Code != 409 {
		t.Fatal("competing writer accepted", got.Code)
	}
}
