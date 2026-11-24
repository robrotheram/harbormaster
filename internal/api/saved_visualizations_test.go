package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
)

func TestSavedVisualizationsArePersonalAndWorkerScoped(t *testing.T) {
	s, rt := setup(t)
	project := domainProject(t, s, "visualizations")
	other := domainProject(t, s, "other")
	ws := domainWorkspace(t, s, "control-token", project.ID, "visualization-worker")
	path := "/v1/projects/" + project.ID
	saved := path + "/analysis/saved-visualizations"
	input := fmt.Sprintf(`{"worker_id":%q,"artifact_id":"heatmap","saved":true}`, ws.ID)
	rename := func(title string) string {
		body, _ := json.Marshal(store.VisualizationDetails{WorkerID: ws.ID, ArtifactID: "heatmap", Title: &title})
		return string(body)
	}
	describe := func(description string) string {
		body, _ := json.Marshal(store.VisualizationDetails{WorkerID: ws.ID, ArtifactID: "heatmap", Description: &description})
		return string(body)
	}
	for _, name := range []string{"alice", "bob"} {
		email := name + "@example.com"
		invite := fmt.Sprintf(`{"email":%q,"role":"member","revision":0,"action":"invite"}`, email)
		if got := request(t, s, "POST", path+"/members", invite); got.Code != 200 {
			t.Fatal(got.Code, got.Body)
		}
		if got := humanRequest(t, s, email, "POST", path+"/accept-invitation", `{"revision":1}`); got.Code != 200 {
			t.Fatal(got.Code, got.Body)
		}
	}

	var reads atomic.Int32
	controller := httptest.NewServer(protocol.NewControllerHandler(protocol.Backend{
		Token:  "runtime-token",
		Health: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) },
		Agent: func(_ context.Context, id, method, route string, _ io.Reader) (int, []byte, *protocol.ControllerError) {
			reads.Add(1)
			if id == ws.ID && method == "GET" && route == "/v1/analysis/artifacts/heatmap" {
				return 200, []byte(`{"schema_version":"1.0","artifact_id":"heatmap","type":"geo","title":"Squirrel density","data":{"reference":"dataset_private"}}`), nil
			}
			return 404, []byte(`{"code":"ANALYSIS_ARTIFACT_UNAVAILABLE"}`), nil
		},
	}))
	t.Cleanup(controller.Close)
	s.Runtime = analysisRuntime{rt, controllerclient.NewClient(controller.URL, "runtime-token")}
	s.cfg.ProjectTokens = map[string]string{"project-token": project.ID}

	if got := request(t, s, "POST", "/v1/projects/"+other.ID+"/analysis/saved-visualizations", input); got.Code != 404 {
		t.Fatal("foreign worker", got.Code, got.Body)
	}
	if got := request(t, s, "POST", "/v1/projects/"+other.ID+"/analysis/saved-visualizations", rename("Foreign title")); got.Code != 404 {
		t.Fatal("foreign worker can be renamed", got.Code, got.Body)
	}
	if got := domainRequest(t, s, "project-token", "GET", saved, ""); got.Code != 403 {
		t.Fatal("project credential can read personal saves", got.Code, got.Body)
	}
	if got := humanRequest(t, s, "eve@example.com", "POST", saved, input); got.Code != 404 {
		t.Fatal("nonmember can save", got.Code, got.Body)
	}
	if reads.Load() != 0 {
		t.Fatal("denied requests reached the worker")
	}

	alice, bob := "alice@example.com", "bob@example.com"
	checkList := func(email string, count int, title string) {
		t.Helper()
		got := humanRequest(t, s, email, "GET", saved, "")
		var result struct {
			Items  []store.SavedVisualization
			Titles []store.VisualizationDetails
		}
		if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &result) != nil || len(result.Items) != count || strings.Contains(got.Body.String(), "dataset_private") {
			t.Fatal("saved list", got.Code, got.Body)
		}
		if count == 1 && result.Items[0] != (store.SavedVisualization{WorkerID: ws.ID, ArtifactID: "heatmap", Title: title, Type: "geo"}) {
			t.Fatal("incorrect saved metadata", result.Items[0])
		}
		if title == "" {
			if len(result.Titles) != 0 {
				t.Fatal("another account's title was exposed", result.Titles)
			}
		} else if len(result.Titles) != 1 || result.Titles[0].WorkerID != ws.ID || result.Titles[0].ArtifactID != "heatmap" ||
			result.Titles[0].Title == nil || *result.Titles[0].Title != title {
			t.Fatal("incorrect custom title", result.Titles)
		}
	}
	for _, title := range []string{"   ", strings.Repeat("A", 201), "Two\nlines"} {
		if got := humanRequest(t, s, alice, "POST", saved, rename(title)); got.Code != 400 {
			t.Fatal("invalid title accepted", got.Code, got.Body)
		}
	}
	if got := humanRequest(t, s, alice, "POST", saved, rename("  Morning observations  ")); got.Code != 200 {
		t.Fatal("rename before saving", got.Code, got.Body)
	}
	checkList(alice, 0, "Morning observations")
	checkList(bob, 0, "")
	const description = "Observations by location.\nCounted in Central Park."
	if got := humanRequest(t, s, alice, "POST", saved, describe(description)); got.Code != 200 {
		t.Fatal("edit description", got.Code, got.Body)
	}
	if got := humanRequest(t, s, alice, "POST", saved, describe(strings.Repeat("A", 2001))); got.Code != 400 {
		t.Fatal("long description accepted", got.Code, got.Body)
	}
	checkDescription := func(email string, want *string) {
		t.Helper()
		got := humanRequest(t, s, email, "GET", saved, "")
		var result struct{ Titles []store.VisualizationDetails }
		if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &result) != nil {
			t.Fatal("description read", got.Code, got.Body)
		}
		if want == nil && len(result.Titles) != 0 ||
			want != nil && (len(result.Titles) != 1 || result.Titles[0].Description == nil || *result.Titles[0].Description != *want) {
			t.Fatal("incorrect description", got.Body)
		}
	}
	wantDescription := description
	checkDescription(alice, &wantDescription)
	checkDescription(bob, nil)
	if got := humanRequest(t, s, alice, "POST", saved, strings.Replace(input, "heatmap", "missing", 1)); got.Code != 404 {
		t.Fatal("missing artifact saved", got.Code, got.Body)
	}
	for range 2 {
		if got := humanRequest(t, s, alice, "POST", saved, input); got.Code != 200 {
			t.Fatal("save", got.Code, got.Body)
		}
	}
	checkList(alice, 1, "Morning observations")
	checkList(bob, 0, "")
	if got := humanRequest(t, s, bob, "POST", saved, describe("Bob's description")); got.Code != 200 {
		t.Fatal("description without a title", got.Code, got.Body)
	}
	if got := humanRequest(t, s, bob, "POST", saved, input); got.Code != 200 {
		t.Fatal("save with a description-only edit", got.Code, got.Body)
	}
	got := humanRequest(t, s, bob, "GET", saved, "")
	var bobList struct {
		Items  []store.SavedVisualization
		Titles []store.VisualizationDetails
	}
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &bobList) != nil ||
		len(bobList.Items) != 1 || bobList.Items[0].Title != "Squirrel density" ||
		len(bobList.Titles) != 1 || bobList.Titles[0].Title != nil {
		t.Fatal("description-only edit hid the original title", got.Code, got.Body)
	}
	remove := strings.Replace(input, "true", "false", 1)
	if got := humanRequest(t, s, bob, "POST", saved, remove); got.Code != 200 {
		t.Fatal("remove by non-owner", got.Code, got.Body)
	}
	if err := s.Store.DeleteStopped(t.Context(), []shared.Workspace{ws}, ""); err != nil {
		t.Fatal(err)
	}
	if got := humanRequest(t, s, alice, "POST", saved, rename("Archived observations")); got.Code != 200 {
		t.Fatal("rename after worker deletion", got.Code, got.Body)
	}
	checkList(alice, 1, "Archived observations")
	checkDescription(alice, &wantDescription)
	if got := humanRequest(t, s, alice, "POST", saved, describe("")); got.Code != 200 {
		t.Fatal("clear description after worker deletion", got.Code, got.Body)
	}
	wantDescription = ""
	checkDescription(alice, &wantDescription)
	if got := humanRequest(t, s, alice, "POST", saved, remove); got.Code != 200 {
		t.Fatal("remove after worker deletion", got.Code, got.Body)
	}
	checkList(alice, 0, "Archived observations")
}
