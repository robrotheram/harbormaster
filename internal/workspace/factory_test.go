package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFactoryArtifactsAndResearch(t *testing.T) {
	cfg := Config{ID: "ws_factory", DataDir: t.TempDir(), Token: "test-workspace-token"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(cfg.DataDir, "files", "report.txt"), []byte("evidence"), 0600)
	body := `{"source_path":"report.txt","idempotency_key":"publish-one"}`
	res := invoke(t, m.Handler(), "POST", "/v1/artifacts", body)
	if res.Code != 201 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	var a Artifact
	json.Unmarshal(res.Body.Bytes(), &a)
	os.WriteFile(filepath.Join(cfg.DataDir, "files", "report.txt"), []byte("edited"), 0600)
	res = invoke(t, m.Handler(), "GET", "/v1/artifacts/"+a.ID+"/content", "")
	if res.Body.String() != "evidence" {
		t.Fatal(res.Body)
	}
	os.Symlink("/etc/passwd", filepath.Join(cfg.DataDir, "files", "escape"))
	res = invoke(t, m.Handler(), "POST", "/v1/artifacts", `{"source_path":"escape","idempotency_key":"escape-one"}`)
	if res.Code < 400 {
		t.Fatal("escaped root")
	}
	res = invoke(t, m.Handler(), "POST", "/v1/recipe-jobs", `{"template":"research-v1","idempotency_key":"research-one"}`)
	if res.Code != 202 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	var j recipeJob
	json.Unmarshal(res.Body.Bytes(), &j)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, err = m.recipe(j.ID)
		if err != nil {
			t.Fatal(err)
		}
		if j.State != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if j.State != "succeeded" {
		b, _ := os.ReadFile(filepath.Join(cfg.DataDir, "files", "outputs", j.ID, "execution.log"))
		t.Fatalf("%+v %s", j, b)
	}
	for _, name := range []string{"baseline.json", "variation.json"} {
		b, err := os.ReadFile(filepath.Join(cfg.DataDir, "files", "outputs", j.ID, name))
		if err != nil {
			t.Fatal(err)
		}
		var metrics map[string]any
		if json.Unmarshal(b, &metrics) != nil || metrics["held_out_mse"] == nil {
			t.Fatal(string(b))
		}
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	res = invoke(t, m.Handler(), "GET", "/v1/artifacts/"+a.ID+"/content", "")
	if res.Body.String() != "evidence" {
		t.Fatal("lost retained artifact")
	}
}

func TestFactoryRestartEvidence(t *testing.T) {
	cfg := Config{ID: "ws_restart", DataDir: t.TempDir(), Token: "test-workspace-token"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.db.Exec(`INSERT INTO recipe_jobs(id,idem_key,template,state,created_at,timeout_seconds) VALUES('job_interrupted','interrupted-one','research-v1','running','2026-11-10T00:00:00Z',600)`)
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	m, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	j, err := m.recipe("job_interrupted")
	if err != nil || j.State != "interrupted" || j.ErrorCode != "SUPERVISOR_RESTART" {
		t.Fatalf("%+v %v", j, err)
	}
}

func TestRecipeCancellationOwnsProcess(t *testing.T) {
	original := researchProgram
	researchProgram = "import time\ntime.sleep(60)\n"
	defer func() { researchProgram = original }()
	cfg := Config{ID: "ws_cancel", DataDir: t.TempDir(), Token: "test-workspace-token"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	res := invoke(t, m.Handler(), "POST", "/v1/recipe-jobs", `{"template":"research-v1","idempotency_key":"long-one"}`)
	if res.Code != 202 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	var j recipeJob
	json.Unmarshal(res.Body.Bytes(), &j)
	if m.Activity().BackgroundJobs != 1 {
		t.Fatal("job absent from idle protection")
	}
	res = invoke(t, m.Handler(), "POST", "/v1/recipe-jobs", `{"template":"research-v1","idempotency_key":"other-one"}`)
	if res.Code != 409 {
		t.Fatal("concurrent writer admitted")
	}
	res = invoke(t, m.Handler(), "POST", "/v1/recipe-jobs/"+j.ID+"/cancel", `{}`)
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, err = m.recipe(j.ID)
		if err != nil {
			t.Fatal(err)
		}
		if j.State == "cancelled" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if j.State != "cancelled" || m.Activity().BackgroundJobs != 0 {
		t.Fatalf("cancellation did not settle: %+v", j)
	}
}
