package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFactoryArtifactsSurviveRestart(t *testing.T) {
	cfg := Config{ID: "ws_factory", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"}
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

func TestWorkspaceCoordinationIsPrivateAndBounded(t *testing.T) {
	cfg := Config{ID: "ws_notes", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	// Empty expected digest creates a new note; a repeated occurrence is safe.
	body := `{"path":"daily.md","body":"Read the paper","expected_digest":"","occurrence":"daily-one","sources":["https://example.com/paper"]}`
	first := invoke(t, m.Handler(), "POST", "/v1/coordination/notes", body)
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	again := invoke(t, m.Handler(), "POST", "/v1/coordination/notes", body)
	if again.Code != 200 || again.Body.String() != first.Body.String() {
		t.Fatal("receipt changed", again.Code)
	}
	if err = os.WriteFile(filepath.Join(cfg.DataDir, "files", "coordination", "notes", "daily.md"), []byte("user edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := invoke(t, m.Handler(), "POST", "/v1/coordination/notes", body); got.Code != 409 {
		t.Fatal("concurrent edit overwritten", got.Code)
	}
	if got := invoke(t, m.Handler(), "POST", "/v1/coordination/notes", `{"path":"../escape.md","body":"escape","occurrence":"escape-one","sources":["https://example.com"]}`); got.Code != 400 {
		t.Fatal("note escaped root", got.Code)
	}
}
