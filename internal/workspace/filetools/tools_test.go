package filetools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesStayConfinedWithoutSupervisor(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	tools := New(root)
	ctx := context.Background()
	if got := tools.Call(ctx, "write_file", json.RawMessage(`{"path":"nested/file","content":"hello"}`)); got != `{"written":true}` {
		t.Fatal(got)
	}
	if got := tools.Call(ctx, "read_file", json.RawMessage(`{"path":"nested/file"}`)); got != `{"content":"hello"}` {
		t.Fatal(got)
	}
	for _, path := range []string{"escape", outside, "../private"} {
		args, _ := json.Marshal(map[string]string{"path": path})
		if got := tools.Call(ctx, "read_file", args); !strings.Contains(got, "FILE_UNAVAILABLE") {
			t.Fatal("escaped filesystem root", path, got)
		}
	}
	for _, args := range []string{`{"path":"nested/file","command":"touch something"}`, `{"path":"nested/file"} {}`} {
		if got := tools.Call(ctx, "read_file", json.RawMessage(args)); !strings.Contains(got, "INVALID_TOOL_ARGUMENTS") {
			t.Fatal("invalid file operation accepted", got)
		}
	}
}
