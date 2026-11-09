package api

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileHandoffIsBoundedAuthorizedAndDoesNotOverwrite(t *testing.T) {
	s, rt := setup(t)
	source, destination := create(t, s, "source"), create(t, s, "destination")
	for _, id := range []string{source.ID, destination.ID} {
		if _, err := rt.Start(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	content := []byte{0, 1, 2, 255, 's', 'e', 'n', 't', 'i', 'n', 'e', 'l'}
	if err := os.WriteFile(filepath.Join(rt.base, source.ID, "files/report.bin"), content, 0600); err != nil {
		t.Fatal(err)
	}
	body := map[string]string{"source_id": source.ID, "destination_id": destination.ID, "source_path": "report.bin", "destination_path": "incoming/report.bin"}
	copyFile := func() int {
		b, _ := json.Marshal(body)
		return request(t, s, "POST", "/v1/file-handoffs", string(b)).Code
	}
	if status := copyFile(); status != 201 {
		t.Fatalf("copy status: %d", status)
	}
	dest := filepath.Join(rt.base, destination.ID, "files/incoming/report.bin")
	if b, err := os.ReadFile(dest); err != nil || !bytes.Equal(b, content) {
		t.Fatal("binary contents changed")
	}
	if copyFile() != 409 {
		t.Fatal("existing destination overwritten")
	}
	body["destination_path"] = "../workspace.db"
	if copyFile() != 400 {
		t.Fatal("path escape accepted")
	}
	body["destination_path"] = "incoming/another.bin"
	body["source_path"] = "../console-home/.codex/auth.json"
	if copyFile() != 400 {
		t.Fatal("credential path accepted")
	}
	body["source_path"] = "report.bin"
	body["destination_id"] = "ws_missing"
	if copyFile() != 404 {
		t.Fatal("unknown destination accepted")
	}
	body["destination_id"] = destination.ID
	rt.unknown = true
	if copyFile() != 409 {
		t.Fatal("unknown activity accepted")
	}
	rt.unknown = false
	// Platform metadata never stores project-file bodies or transfer paths.
	rows, _ := s.Store.Workspaces(context.Background())
	metadata, _ := json.Marshal(rows)
	if bytes.Contains(metadata, content) || bytes.Contains(metadata, []byte("report.bin")) {
		t.Fatal("content leaked into metadata")
	}
}
