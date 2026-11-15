package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"agent-workbench/internal/shared"
)

type projectNamespaceFake struct {
	*embeddedRuntime
	fail  bool
	calls []string
}

func (r *projectNamespaceFake) EnsureProjectNamespace(_ context.Context, id string) (string, error) {
	r.calls = append(r.calls, id)
	if r.fail {
		return "", errors.New("cluster unavailable")
	}
	return shared.ProjectNamespace(id), nil
}
func TestProjectNamespaceFailureKeepsCreationIdentityAndSharedWorkspace(t *testing.T) {
	s, embedded := setup(t)
	rt := &projectNamespaceFake{embeddedRuntime: embedded, fail: true}
	s.Runtime = rt
	s.cfg.ProvisionProjectNamespaces = true
	s.cfg.SharedWorkspaceID = "ws_shared"
	if _, err := s.EnsureSharedWorkspace(context.Background()); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"Applications","idempotency_key":"project-create"}`
	response := request(t, s, "POST", "/v1/projects", body)
	if response.Code != 503 {
		t.Fatal(response.Code, response.Body)
	}
	p, err := s.Store.ProjectByKey(context.Background(), "project-create")
	if err != nil || p.DeploymentNamespace == "" || p.NamespaceReady {
		t.Fatal("failed setup lost durable project", p, err)
	}
	rt.fail = false
	response = request(t, s, "POST", "/v1/projects", body)
	var ready shared.Project
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &ready) != nil || ready.ID != p.ID || !ready.NamespaceReady {
		t.Fatal("retry allocated another project", response.Code, response.Body)
	}
	response = request(t, s, "POST", "/v1/projects/"+ready.ID+"/namespace", `{}`)
	if response.Code != 200 {
		t.Fatal("namespace repair unavailable", response.Code, response.Body)
	}
	response = request(t, s, "POST", "/v1/projects", `{"name":"Other","idempotency_key":"other"}`)
	var other shared.Project
	if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &other) != nil || other.DeploymentNamespace == ready.DeploymentNamespace {
		t.Fatal("projects share deployment namespace", response.Code, response.Body)
	}
	workers, err := s.Store.Workspaces(context.Background())
	if err != nil || len(workers) != 1 || workers[0].ID != "ws_shared" || embedded.starts != 0 {
		t.Fatal("projects allocated compute", workers, err)
	}
	if response = request(t, s, "POST", "/v1/projects/default/namespace", `{}`); response.Code != 409 {
		t.Fatal("legacy namespace provisioned", response.Code)
	}
	if response = request(t, s, "POST", "/v1/projects/"+ready.ID+"/delete", `{"confirm_id":"`+ready.ID+`"}`); response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
	if response = request(t, s, "POST", "/v1/projects/"+ready.ID+"/namespace", `{}`); response.Code != 404 {
		t.Fatal("deleted project reprovisioned", response.Code)
	}
}
