package previewsvc

import (
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace"
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testLifecycle struct{}

func (testLifecycle) Start(_ context.Context, ws *shared.Workspace) error {
	ws.State = "READY"
	return nil
}

func (testLifecycle) Stop(context.Context, *shared.Workspace) error { return nil }

func TestPromotionThroughControllerClientAndRealWorkspaceHandlers(t *testing.T) {
	managers := map[string]*workspace.Manager{}
	directories := map[string]string{}
	for _, id := range []string{"ws_coding", "ws_retained"} {
		directory := t.TempDir()
		directories[id] = directory
		m, err := workspace.New(workspace.Config{ID: id, HomeID: "home_" + id, DataDir: directory, Token: "guest-token"})
		if err != nil {
			t.Fatal(err)
		}
		managers[id] = m
		defer func() {
			if managers[id] != nil {
				managers[id].Close()
			}
		}()
	}
	// Wire adapter substitutes only controller allocation discovery. Guest routes,
	// authentication, persistence and controllerclient serialization are real.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer controller-token" {
			w.WriteHeader(401)
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/v1/workspaces/"), "/agent", 2)
		if len(parts) != 2 || managers[parts[0]] == nil {
			w.WriteHeader(404)
			return
		}
		r.URL.Path = parts[1]
		r.Header.Set("Authorization", "Bearer guest-token")
		managers[parts[0]].Handler().ServeHTTP(w, r)
	}))
	defer server.Close()
	client := controllerclient.NewClient(server.URL, "controller-token")
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	tw.WriteHeader(&tar.Header{Name: "site/index.html", Mode: 0600, Size: 14})
	tw.Write([]byte("<h1>hello</h1>"))
	tw.Close()
	os.WriteFile(filepath.Join(directories["ws_coding"], "files", "gallery.tar"), b.Bytes(), 0600)
	status, body, err := client.Agent(context.Background(), "ws_coding", "POST", "/v1/artifacts", []byte(`{"source_path":"gallery.tar","idempotency_key":"archive-one"}`))
	if err != nil || status != 201 {
		t.Fatalf("artifact %d %s %v", status, body, err)
	}
	var artifact struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &artifact)
	coding := shared.Workspace{ID: "ws_coding", ProjectID: "proj_one", HomeID: "home_coding", State: "READY"}
	retained := shared.Workspace{ID: "ws_retained", ProjectID: "proj_one", HomeID: "home_retained", State: "STOPPED"}
	svc := Service{Runtime: client, Lifecycle: testLifecycle{}}
	if _, err := svc.Retain(context.Background(), coding, &coding, artifact.ID, 3600, "retain-one"); err == nil {
		t.Fatal("same allocation admitted")
	}
	foreign := retained
	foreign.ProjectID = "proj_other"
	if _, err := svc.Retain(context.Background(), coding, &foreign, artifact.ID, 3600, "retain-one"); err == nil {
		t.Fatal("cross-project promotion admitted")
	}
	body, err = svc.Retain(context.Background(), coding, &retained, artifact.ID, 3600, "retain-one")
	if err != nil {
		t.Fatal(err)
	}
	managers["ws_coding"].Close()
	delete(managers, "ws_coding")
	var service struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &service)
	status, body, err = client.Agent(context.Background(), retained.ID, "POST", "/v1/services/"+service.ID+"/fetch", []byte(`{"path":"index.html"}`))
	if err != nil || status != 200 {
		t.Fatalf("%d %s %v", status, body, err)
	}
}
