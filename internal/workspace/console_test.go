package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInitialTitleUsesLiveHTTPAndPreservesLaterRename(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/projects/mutate" || r.Header.Get("Authorization") != "Bearer test-grant" || r.Header.Get("x-t3-orchestration-protocol") != "2" {
			t.Errorf("unexpected live project request")
		}
		var command map[string]string
		if err := json.NewDecoder(r.Body).Decode(&command); err != nil || command["type"] != "project.update" || command["projectId"] != "project-one" || command["title"] != "Photo gallery" {
			t.Errorf("unexpected project mutation: %v", command)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	c := &console{home: t.TempDir(), title: "Photo gallery"}
	c.initializeTitle(context.Background(), server.URL, "test-grant", "project-one")
	if _, err := os.Stat(filepath.Join(c.home, ".initial-project-title")); err != nil {
		t.Fatal(err)
	}
	c.title = "Do not overwrite the user's later rename"
	c.initializeTitle(context.Background(), server.URL, "test-grant", "project-one")
	if calls != 1 {
		t.Fatalf("got %d mutations; want one", calls)
	}
}
