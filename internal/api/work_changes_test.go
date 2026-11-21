package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"agent-workbench/internal/shared"
)

func TestWorkAttemptShowsRetainedGitChanges(t *testing.T) {
	s, rt := setup(t)
	project := domainProject(t, s, "work-changes")
	other := domainProject(t, s, "other-project")
	ws := domainWorkspace(t, s, "control-token", project.ID, "changes")
	files := filepath.Join(rt.base, ws.ID, "files")
	write := func(name, content string) error {
		return os.WriteFile(filepath.Join(files, name), []byte(content), 0600)
	}
	inference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for name, content := range map[string]string{
			"existing.txt":         "revised\n",
			":new file.txt":        "created\n",
			"nested/page.html":     "<h1>Revised</h1>\n",
			"ignored.txt":          "ignored-secret\n",
			".env":                 "env-secret\n",
			"work-requests/run.md": "private-transport\n",
		} {
			if err := write(name, content); err != nil {
				t.Error(err)
			}
		}
		if err := os.Remove(filepath.Join(files, "delete.txt")); err != nil {
			t.Error(err)
		}
		if err := os.Remove(filepath.Join(files, "old-dir/page.html")); err != nil {
			t.Error(err)
		}
		if err := os.Remove(filepath.Join(files, "old-dir")); err != nil {
			t.Error(err)
		}
		if err := os.Symlink("nested", filepath.Join(files, "old-dir")); err != nil {
			t.Error(err)
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"Done"}}]}`)
	}))
	defer inference.Close()
	rt.upstream = inference.URL
	if got := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"start"}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	for name, content := range map[string]string{
		"existing.txt": "original\n",
		"delete.txt":   "removed\n",
		".gitignore":   "ignored.txt\n",
	} {
		if err := write(name, content); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = files
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s (%v)", args, out, err)
		}
		return string(out)
	}
	git("add", "existing.txt")
	git("init", "--quiet", "nested")
	if err := write("nested/page.html", "<h1>Original</h1>\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(files, "old-dir"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := write("old-dir/page.html", "<h1>Old file</h1>\n"); err != nil {
		t.Fatal(err)
	}
	git("add", "old-dir/page.html")
	var item shared.WorkItem
	created := request(t, s, "POST", "/v1/work-items", `{"project_id":"`+project.ID+`","workspace_id":"`+ws.ID+`","instruction":"Update the project files.","kind":"software","idempotency_key":"create"}`)
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &item) != nil {
		t.Fatal(created.Code, created.Body.String())
	}
	path := "/v1/work-items/" + item.ID
	scope := "?project_id=" + project.ID
	var attempt shared.WorkAttempt
	run := request(t, s, "POST", path+"/run", `{"project_id":"`+project.ID+`","idempotency_key":"attempt"}`)
	if run.Code != 202 || json.Unmarshal(run.Body.Bytes(), &attempt) != nil {
		t.Fatal(run.Code, run.Body.String())
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		s.tickOrchestration(context.Background())
		got := request(t, s, "GET", path+scope, "")
		if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &item) != nil {
			t.Fatal(got.Code, got.Body.String())
		}
		if item.State == "awaiting_review" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if item.State != "awaiting_review" {
		t.Fatalf("task did not finish: %s", item.State)
	}
	if indexed := git("show", ":existing.txt"); indexed != "original\n" {
		t.Fatalf("the task comparison modified the project index: %q", indexed)
	}
	// Later working-tree edits and restarting the worker must leave the patch intact.
	if err := write("existing.txt", "later\n"); err != nil {
		t.Fatal(err)
	}
	if err := rt.Stop(context.Background(), ws.ID); err != nil {
		t.Fatal(err)
	}
	changesPath := path + "/attempts/" + attempt.ID + "/changes"
	got := request(t, s, "GET", changesPath+scope, "")
	var changes shared.TaskChanges
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &changes) != nil || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(got.Code, got.Body.String())
	}
	want := []shared.TaskFileChange{
		{Path: ":new file.txt", Status: "A"},
		{Path: "delete.txt", Status: "D"},
		{Path: "existing.txt", Status: "M"},
		{Path: "nested/page.html", Status: "M"},
		{Path: "old-dir", Status: "A"},
		{Path: "old-dir/page.html", Status: "D"},
	}
	sources := map[string]string{
		":new file.txt":     "created\n",
		"delete.txt":        "removed\n",
		"existing.txt":      "revised\n",
		"nested/page.html":  "<h1>Revised</h1>\n",
		"old-dir":           "nested",
		"old-dir/page.html": "<h1>Old file</h1>\n",
	}
	for i := range changes.Files {
		file := &changes.Files[i]
		if file.Source == nil || string(*file.Source) != sources[file.Path] {
			t.Errorf("incorrect retained source for %q: %v", file.Path, file.Source)
		}
		file.Source = nil
	}
	if !reflect.DeepEqual(changes.Files, want) || changes.Truncated {
		t.Fatalf("unexpected changed files: %+v", changes.Files)
	}
	for _, line := range []string{"+created", "-removed", "-original", "+revised"} {
		if !strings.Contains(changes.Diff, line+"\n") {
			t.Errorf("diff is missing %q: %s", line, changes.Diff)
		}
	}
	if got := request(t, s, "GET", changesPath+"?project_id="+other.ID, ""); got.Code != 404 {
		t.Fatalf("another project could open the changes: %d %s", got.Code, got.Body.String())
	}
}
