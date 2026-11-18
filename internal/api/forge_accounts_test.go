package api

import (
	"agent-workbench/internal/store"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountCreateRotateAndEncryptedPersistence(t *testing.T) {
	s, _ := setup(t)
	t.Setenv("WORKBENCH_FORGE_CONFIG", "")
	calls := 0
	repositoryAllowed := false
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.EscapedPath() == "/api/v4/projects/team%2Frepo/issues" {
			if r.Method != http.MethodGet || r.Header.Get("PRIVATE-TOKEN") != "synthetic-private-token" {
				t.Error("repository access check must use the saved provider credential")
			}
			if !repositoryAllowed {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"private provider detail"}`))
				return
			}
			w.Write([]byte(`[]`))
			return
		}
		if r.URL.Path != "/api/v4/user" {
			t.Error(r.URL.Path)
		}
		if r.Header.Get("PRIVATE-TOKEN") == "rejected" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":"rejected secret"}`))
			return
		}
		w.Write([]byte(`{"id":123}`))
	}))
	defer remote.Close()
	previous := http.DefaultTransport
	http.DefaultTransport = remote.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = previous })
	save := func(token string, revision int, address string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"name": "Office GitLab", "provider": "gitlab", "api_url": address, "token": token, "revision": revision})
		return request(t, s, "POST", "/v1/forge-hosts", string(body))
	}
	response := save("rejected", 0, remote.URL)
	if response.Code != 502 || strings.Contains(response.Body.String(), "rejected secret") {
		t.Fatal(response.Code, response.Body)
	}
	accounts, _ := s.Store.ForgeAccounts(context.Background())
	if len(accounts) != 0 {
		t.Fatal("rejected account was saved")
	}
	response = save("synthetic-private-token", 0, remote.URL)
	if response.Code != 200 || strings.Contains(response.Body.String(), "synthetic-private-token") || strings.Contains(response.Body.String(), `"token"`) {
		t.Fatal(response.Code, response.Body)
	}
	var account store.ForgeAccount
	json.Unmarshal(response.Body.Bytes(), &account)
	if account.Revision != 1 || !account.Managed || account.APIURL != remote.URL+"/api/v4" {
		t.Fatal(account)
	}
	var metadata, credential string
	if err := s.Store.DB.QueryRow(`SELECT metadata,credential FROM forge_accounts`).Scan(&metadata, &credential); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata+credential, "synthetic-private-token") {
		t.Fatal("plaintext credential persisted")
	}
	hosts, err := s.forgeHosts(context.Background())
	if err != nil || len(hosts) != 1 || hosts[0].Token != "synthetic-private-token" {
		t.Fatal("stored credential unavailable", err)
	}
	connect := func() *httptest.ResponseRecorder {
		return request(t, s, "POST", "/v1/forge-connections", `{"host":"Office GitLab","repository":"team/repo"}`)
	}
	response = connect()
	if response.Code != 502 || !strings.Contains(response.Body.String(), `"code":"FORGE_HTTP_403"`) || strings.Contains(response.Body.String(), "private provider detail") {
		t.Fatal(response.Code, response.Body)
	}
	connections, err := s.Store.ForgeConnections(context.Background())
	if err != nil || len(connections) != 0 {
		t.Fatal("repository was saved after provider denied access", err)
	}
	repositoryAllowed = true
	if response = connect(); response.Code != 201 {
		t.Fatal(response.Code, response.Body)
	}
	response = save("rejected", 1, remote.URL)
	if response.Code != 502 {
		t.Fatal(response.Code, response.Body)
	}
	hosts, err = s.forgeHosts(context.Background())
	if err != nil || hosts[0].Token != "synthetic-private-token" {
		t.Fatal("failed update changed saved token", err)
	}
	count := calls
	response = save("synthetic-private-token", 0, remote.URL)
	if response.Code != 409 || calls != count {
		t.Fatal("duplicate create contacted provider")
	}
	response = save("new-token", 1, "https://different.example")
	if response.Code != 409 || calls != count {
		t.Fatal("account destination changed")
	}
	response = save("new-token", 1, remote.URL)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
	hosts, err = s.forgeHosts(context.Background())
	if err != nil || hosts[0].Token != "new-token" {
		t.Fatal("rotation failed", err)
	}
	response = request(t, s, "GET", "/v1/forge-hosts", "")
	if response.Code != 200 || strings.Contains(response.Body.String(), "new-token") {
		t.Fatal("secret in listing")
	}
	// A server-key rotation leaves the listing usable so the operator can reauthenticate.
	s.cfg.Token = "rotated-control-token"
	if _, err = s.forgeHosts(context.Background()); err == nil {
		t.Fatal("wrong encryption key accepted")
	}
	if _, err = s.forgeAccountSummaries(context.Background()); err != nil {
		t.Fatal("cannot list accounts to recover", err)
	}
}
