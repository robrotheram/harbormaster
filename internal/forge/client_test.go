package forge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviders(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if provider == "github" {
					if r.Header.Get("Authorization") != "Bearer secret" || r.URL.Path != "/repos/team/repo/issues" {
						t.Errorf("bad github request")
					}
					w.Write([]byte(`[{"number":1,"title":"Bug","html_url":"https://github.com/team/repo/issues/1","labels":[{"name":"bug"}]},{"number":2,"pull_request":{}}]`))
				} else {
					if r.Header.Get("PRIVATE-TOKEN") != "secret" || !strings.Contains(r.RequestURI, "team%2Frepo") {
						t.Errorf("bad gitlab request: %s", r.RequestURI)
					}
					w.Write([]byte(`[{"iid":1,"title":"Bug","description":"Fix it","state":"opened","labels":["bug"],"milestone":{"title":"Sprint 1"}}]`))
				}
			}))
			defer server.Close()
			items, err := (Client{Host: Host{Provider: provider, APIURL: server.URL, Token: "secret"}}).List(context.Background(), "team/repo", 1)
			if err != nil || len(items) != 1 || items[0].Number != 1 || items[0].Labels[0] != "bug" {
				t.Fatalf("%+v %v", items, err)
			}
		})
	}
}
func TestRedirectAndErrorRedaction(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect followed with credential") }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	_, err := (Client{Host: Host{Provider: "github", APIURL: server.URL, Token: "secret"}}).List(context.Background(), "team/repo", 1)
	if err == nil || err.Error() != "FORGE_HTTP_302" {
		t.Fatal(err)
	}
}
func TestConfigAndRepositoryValidation(t *testing.T) {
	for _, repo := range []string{"../repo", "owner/repo?token=x", "owner//repo", "owner/repo/../other"} {
		if ValidRepository("gitlab", repo) {
			t.Fatal(repo)
		}
	}
	if !ValidRepository("gitlab", "group/sub/repo") || ValidRepository("github", "group/sub/repo") {
		t.Fatal("nested group validation")
	}
	path := filepath.Join(t.TempDir(), "hosts.json")
	t.Setenv("WORKBENCH_FORGE_CONFIG", path)
	os.WriteFile(path, []byte(`[{"name":"office","provider":"gitlab","api_url":"http://private/api/v4","token":"secret"}]`), 0600)
	if _, err := Hosts(); err == nil {
		t.Fatal("accepted plaintext credential transport")
	}
}
func TestProviderStateUpdates(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method := "PATCH"
				if provider == "gitlab" {
					method = "PUT"
				}
				if r.Method != method {
					t.Error(r.Method)
				}
				w.Write([]byte(`{"number":7,"iid":7,"state":"closed"}`))
			}))
			defer server.Close()
			item, err := (Client{Host: Host{Provider: provider, APIURL: server.URL, Token: "secret"}}).Save(context.Background(), "team/repo", 7, "", "", "closed")
			if err != nil || item.Number != 7 || item.State != "closed" {
				t.Fatal(item, err)
			}
		})
	}
}

func TestDescriptionUpdatesAndOmission(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		t.Run(provider, func(t *testing.T) {
			var received map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received = nil
				json.NewDecoder(r.Body).Decode(&received)
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"number":1,"iid":1,"title":"Issue","state":"closed"}`))
			}))
			defer server.Close()
			c := Client{Host: Host{Provider: provider, APIURL: server.URL, Token: "test"}}
			field := "body"
			if provider == "gitlab" {
				field = "description"
			}
			for _, body := range []string{"## Summary\n- [ ] Task", ""} {
				if _, err := c.SaveDescription(context.Background(), "team/repo", 1, "Issue", &body, "closed"); err != nil {
					t.Fatal(err)
				}
				if received[field] != body {
					t.Fatal(received)
				}
			}
			if _, err := c.Save(context.Background(), "team/repo", 1, "Issue", "", "closed"); err != nil {
				t.Fatal(err)
			}
			if _, exists := received[field]; exists {
				t.Fatal("old title-only save erased description")
			}
		})
	}
}
