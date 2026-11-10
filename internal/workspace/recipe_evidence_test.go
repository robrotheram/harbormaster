package workspace

import (
	"encoding/json"
	"testing"
	"time"
)

func TestResearchAutomaticallyArchivesEvidenceBeforeSuccess(t *testing.T) {
	cfg := Config{ID: "ws_evidence", DataDir: t.TempDir(), Token: "test-workspace-token"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	res := invoke(t, m.Handler(), "POST", "/v1/recipe-jobs", `{"template":"research-v1","idempotency_key":"evidence-research"}`)
	if res.Code != 202 {
		m.Close()
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	var job recipeJob
	json.Unmarshal(res.Body.Bytes(), &job)
	deadline := time.Now().Add(10 * time.Second)
	for {
		job, err = m.recipe(job.ID)
		if err != nil {
			m.Close()
			t.Fatal(err)
		}
		if job.State != "running" {
			break
		}
		if time.Now().After(deadline) {
			m.Close()
			t.Fatal("research did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.State != "succeeded" {
		m.Close()
		t.Fatalf("%+v", job)
	}
	var id string
	err = m.db.QueryRow(`SELECT id FROM artifacts WHERE source_path=?`, "outputs/"+job.ID+"/baseline.json").Scan(&id)
	if err != nil {
		m.Close()
		t.Fatal("success without archived evidence", err)
	}
	m.Close()
	m, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	res = invoke(t, m.Handler(), "GET", "/v1/artifacts/"+id+"/content", "")
	if res.Code != 200 || !json.Valid(res.Body.Bytes()) {
		t.Fatalf("archived evidence unreadable: %d %s", res.Code, res.Body)
	}
}
