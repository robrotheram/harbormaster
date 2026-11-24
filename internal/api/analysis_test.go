package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agent-workbench/internal/dashboard"
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
)

type analysisRuntime struct {
	*embeddedRuntime
	Client *controllerclient.Client
}

func (a analysisRuntime) Agent(ctx context.Context, id, method, path string, body []byte) (int, []byte, error) {
	return a.Client.Agent(ctx, id, method, path, body)
}

func (a analysisRuntime) UploadDataset(ctx context.Context, id, path string, body io.Reader) (int, []byte, error) {
	return a.Client.UploadDataset(ctx, id, path, body)
}

// Use the real dashboard, control-plane and controller boundaries. The
// controller's test backend signals after receiving data while the upload is
// still open, which catches accidental buffering anywhere in that chain.
func TestAnalysisGatewayStreamsAndAuthorizes(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "data-owner")
	ctx := t.Context()
	if err := s.Store.CreateProject(ctx, shared.Project{ID: "foreign", Name: "Foreign", CreatedAt: s.now()}, "foreign"); err != nil {
		t.Fatal(err)
	}
	cross := shared.Workspace{ID: "ws_foreign", ProjectID: "foreign", HomeID: "home_foreign", AgentKind: "codex", State: "STOPPED", CreatedAt: s.now()}
	if err := s.Store.Create(ctx, cross, "other-worker"); err != nil {
		t.Fatal(err)
	}
	s.cfg.ProjectTokens = map[string]string{"foreign-token": "foreign"}
	started := make(chan string, 1)
	var calls atomic.Int32
	controller := httptest.NewServer(protocol.NewControllerHandler(protocol.Backend{
		Token:  "runtime-token",
		Health: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) },
		UploadDataset: func(_ context.Context, id, path string, data io.Reader) (int, []byte, *protocol.ControllerError) {
			calls.Add(1)
			if id != ws.ID || path != "/v1/analysis/datasets/upload?name=sensor+data.parquet" {
				return 0, nil, protocol.Failure(400, "BAD_TEST_ROUTE")
			}
			first := make([]byte, 8)
			if _, err := io.ReadFull(data, first); err != nil {
				return 0, nil, protocol.Failure(400, "BAD_TEST_BODY")
			}
			started <- string(first)
			n, err := io.Copy(io.Discard, data)
			if err != nil || n != 4<<20 {
				return 0, nil, protocol.Failure(400, "BAD_TEST_BODY")
			}
			return 201, []byte(`{"dataset_id":"dataset_test"}`), nil
		},
		Agent: func(_ context.Context, id, method, path string, _ io.Reader) (int, []byte, *protocol.ControllerError) {
			calls.Add(1)
			if id == ws.ID && method == "GET" && path == "/v1/analysis/datasets/dataset_test/schema" {
				return 200, []byte(`{"dataset_id":"dataset_test"}`), nil
			}
			return 404, []byte(`{"code":"ANALYSIS_DATA_UNAVAILABLE"}`), nil
		},
	}))
	t.Cleanup(controller.Close)
	s.Runtime = analysisRuntime{rt, controllerclient.NewClient(controller.URL, "runtime-token")}

	for _, test := range []struct {
		method, url, token string
		status             int
	}{
		{"GET", ws.ID + "/analysis/datasets", "", 401},
		{"GET", ws.ID + "/analysis/datasets", "foreign-token", 404},
		{"POST", ws.ID + "/analysis/datasets/upload?name=sensor.csv", "foreign-token", 404},
		{"GET", ws.ID + "/analysis/datasets?project_id=foreign", "control-token", 404},
		{"GET", ws.ID + "/analysis/datasets/a%2Fb/schema", "control-token", 404},
		{"POST", ws.ID + "/analysis/results/upload?name=sensor.csv", "control-token", 404},
	} {
		r := httptest.NewRequest(test.method, "/v1/workspaces/"+test.url, nil)
		r.Header.Set("Authorization", "Bearer "+test.token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%s %s: received %d, want %d (%s)", test.method, test.url, w.Code, test.status, w.Body)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("denied request reached runtime")
	}

	control := httptest.NewServer(s.Handler())
	t.Cleanup(control.Close)
	ingress, err := dashboard.NewInteractiveHandler(control.URL, control.URL, "control-token")
	if err != nil {
		t.Fatal(err)
	}
	ui := httptest.NewServer(ingress)
	t.Cleanup(ui.Close)
	pipeRead, pipeWrite := io.Pipe()
	defer pipeWrite.Close()
	req, err := http.NewRequestWithContext(ctx, "POST", ui.URL+"/ui-api/workspaces/"+ws.ID+"/analysis/datasets/upload?name=sensor+data.parquet&project_id=default", pipeRead)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Workbench-Request", "1")
	req.Header.Set("Content-Type", "application/octet-stream")
	type uploadResult struct {
		response *http.Response
		err      error
	}
	done := make(chan uploadResult, 1)
	go func() {
		resp, err := ui.Client().Do(req)
		done <- uploadResult{resp, err}
	}()
	if _, err := io.WriteString(pipeWrite, "PAR1test"); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-started:
		if value != "PAR1test" {
			t.Fatalf("upload bytes changed: %q", value)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("the controller did not receive data before upload completion")
	}
	for range 64 {
		if _, err := io.WriteString(pipeWrite, strings.Repeat("0", 64<<10)); err != nil {
			t.Fatal(err)
		}
	}
	_ = pipeWrite.Close()
	result := <-done
	if result.err != nil {
		t.Fatal(result.err)
	}
	defer result.response.Body.Close()
	var metadata map[string]string
	if result.response.StatusCode != 201 || json.NewDecoder(result.response.Body).Decode(&metadata) != nil || metadata["dataset_id"] != "dataset_test" {
		t.Fatalf("unexpected upload response %d, %v", result.response.StatusCode, metadata)
	}

	path := "/v1/workspaces/%s/analysis/datasets/dataset_test/schema"
	r := request(t, s, "GET", strings.Replace(path, "%s", ws.ID, 1), "")
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body)
	}
	r = request(t, s, "GET", strings.Replace(path, "%s", cross.ID, 1), "")
	if r.Code != 404 {
		t.Fatal(r.Code, r.Body)
	}
	r = request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/delete", `{"confirm_id":"`+ws.ID+`"}`)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body)
	}
	r = request(t, s, "GET", strings.Replace(path, "%s", ws.ID, 1), "")
	if r.Code != 404 || calls.Load() != 3 {
		t.Fatalf("deleted worker still routable: %d, controller calls %d", r.Code, calls.Load())
	}
}
