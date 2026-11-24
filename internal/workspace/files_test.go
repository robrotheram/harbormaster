package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agent-workbench/internal/shared"
)

func TestFileHandoffRejectsEscapingSymlinksAndOversize(t *testing.T) {
	dir := t.TempDir()
	m, err := New(Config{ID: "ws_test", HomeID: "home_test", DataDir: dir, Token: "test-workspace-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	out := t.TempDir()
	if err = os.WriteFile(filepath.Join(out, "secret"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(out, filepath.Join(dir, "files/escape")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "files/.env"), []byte("private fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(".env", filepath.Join(dir, "files/local-link")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"escape/secret", "../workspace.db", "local-link"} {
		body, _ := json.Marshal(shared.FileHandoff{Path: p})
		if invoke(t, m.Handler(), "POST", "/v1/files/read", string(body)).Code < 400 {
			t.Fatal("escaping read accepted")
		}
		body, _ = json.Marshal(shared.FileHandoff{Path: p, Content: []byte("overwrite")})
		if invoke(t, m.Handler(), "POST", "/v1/files/write-new", string(body)).Code < 400 {
			t.Fatal("escaping write accepted")
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "files/large"), make([]byte, shared.HandoffLimit+1), 0600); err != nil {
		t.Fatal(err)
	}
	if invoke(t, m.Handler(), "POST", "/v1/files/read", `{"path":"large"}`).Code != 400 {
		t.Fatal("oversize file accepted")
	}
	b, _ := os.ReadFile(filepath.Join(out, "secret"))
	if string(b) != "private" {
		t.Fatal("outside file changed")
	}
}
