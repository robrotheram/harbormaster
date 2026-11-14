package api

import (
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForgeHostCredentialsAreRedacted(t *testing.T) {
	s, _ := setup(t)
	path := filepath.Join(t.TempDir(), "hosts.json")
	os.WriteFile(path, []byte(`[{"name":"office","provider":"gitlab","api_url":"https://gitlab.example/api/v4","token":"never-return-this"}]`), 0600)
	t.Setenv("WORKBENCH_FORGE_CONFIG", path)
	result := request(t, s, "GET", "/v1/forge-hosts", "")
	if result.Code != 200 || strings.Contains(result.Body.String(), "never-return-this") || strings.Contains(result.Body.String(), `"token"`) {
		t.Fatal(result.Code, result.Body)
	}
}
func TestForgeConnectionsArePlatformResources(t *testing.T) {
	s, _ := setup(t)
	if err := s.Store.SaveForge(context.Background(), store.ForgeConnection{ID: "forge_one", Host: "office", Repository: "team/repo"}); err != nil {
		t.Fatal(err)
	}
	response := request(t, s, "GET", "/v1/forge-connections", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), "forge_one") || strings.Contains(response.Body.String(), "project_id") {
		t.Fatal(response.Code, response.Body)
	}
	response = request(t, s, "POST", "/v1/forge-connections/forge_one/issues/1/import", `{}`)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "PROJECT_REQUIRED") {
		t.Fatal(response.Code, response.Body)
	}
	response = request(t, s, "POST", "/v1/forge-connections/forge_one/delete", `{}`)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
}

func TestForgeImportIsPrivateDraftAndDeduplicated(t *testing.T) {
	s, _ := setup(t)
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "private-token" {
			t.Error("missing authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"iid":12,"title":"Fix issue","description":"Private requirement","web_url":"https://example.test/issue/12","state":"opened"}`))
	}))
	defer remote.Close()
	previous := http.DefaultTransport
	http.DefaultTransport = remote.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = previous })
	path := filepath.Join(t.TempDir(), "hosts.json")
	hosts, _ := json.Marshal([]map[string]string{{"name": "office", "provider": "gitlab", "api_url": remote.URL + "/api/v4", "token": "private-token"}})
	os.WriteFile(path, hosts, 0600)
	t.Setenv("WORKBENCH_FORGE_CONFIG", path)
	if err := s.Store.SaveForge(context.Background(), store.ForgeConnection{ID: "forge_import", Host: "office", Repository: "team/repo"}); err != nil {
		t.Fatal(err)
	}
	var first shared.WorkItem
	for i, status := range []int{201, 200} {
		response := request(t, s, "POST", "/v1/forge-connections/forge_import/issues/12/import", `{"project_id":"default"}`)
		var item shared.WorkItem
		if response.Code != status || json.Unmarshal(response.Body.Bytes(), &item) != nil {
			t.Fatal(response.Code, response.Body)
		}
		if item.State != "draft" || strings.Contains(response.Body.String(), "private-token") || strings.Contains(response.Body.String(), "Private requirement") {
			t.Fatal(response.Body)
		}
		if i == 0 {
			first = item
		} else if first.ID != item.ID {
			t.Fatal("duplicate work created")
		}
	}
	response := request(t, s, "GET", "/v1/work-items/"+first.ID+"/instruction?project_id=default", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), "Private requirement") {
		t.Fatal(response.Code, response.Body)
	}
}

func TestPlatformConnectionsRequireAdministrator(t *testing.T) {
	s, _ := setup(t)
	project := domainProject(t, s, "repo-reader")
	s.cfg.ProjectTokens = map[string]string{"project-reader": project.ID}
	for _, test := range []struct{ method, path, body string }{
		{"GET", "/v1/forge-hosts", ""},
		{"POST", "/v1/forge-hosts", `{}`},
		{"GET", "/v1/forge-connections", ""},
		{"POST", "/v1/forge-connections", `{"host":"office","repository":"team/repo"}`},
		{"GET", "/v1/forge-connections/forge_one/issues", ""},
		{"POST", "/v1/forge-connections/forge_one/delete", `{}`},
	} {
		result := domainRequest(t, s, "project-reader", test.method, test.path, test.body)
		if result.Code != 403 {
			t.Fatal(test.path, result.Code, result.Body)
		}
	}
}
