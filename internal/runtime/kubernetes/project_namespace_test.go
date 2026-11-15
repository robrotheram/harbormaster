package kubernetes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"agent-workbench/internal/shared"
)

func TestApplicationNamespaceProvisioningRetriesAndRejectsForeignOwnership(t *testing.T) {
	k, _ := kubeProvider(t)
	k.cfg.ProvisionProjectNamespaces = true
	objects := map[string]map[string]any{}
	posts := 0
	unavailable := true
	k.apiHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status := 200
		var body any
		path := r.URL.Path
		if path == "/api/v1/namespaces/harbormaster/secrets/harbormaster-registry" {
			if unavailable {
				status = 503
			} else {
				body = map[string]any{"type": "kubernetes.io/dockerconfigjson", "data": map[string]any{".dockerconfigjson": "e30="}}
			}
		} else if r.Method == "GET" {
			var ok bool
			body, ok = objects[path]
			if !ok {
				status = 404
			}
		} else if r.Method == "POST" {
			var obj map[string]any
			if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
				t.Fatal(err)
			}
			if obj["kind"] == "Sandbox" || obj["kind"] == "Pod" {
				t.Fatal("project created agent compute")
			}
			meta := obj["metadata"].(map[string]any)
			key := path + "/" + meta["name"].(string)
			if objects[key] != nil {
				status = 409
			} else {
				objects[key] = obj
				posts++
				status = 201
			}
			body = obj
		} else {
			t.Fatalf("unexpected mutation %s %s", r.Method, path)
		}
		b, _ := json.Marshal(body)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})}
	project := "prj_" + strings.Repeat("a", 32)
	ns := shared.ProjectNamespace(project)
	if _, err := k.EnsureProjectNamespace(context.Background(), project); err == nil {
		t.Fatal("registry outage hidden")
	}
	if posts != 4 {
		t.Fatal("unexpected partial provisioning", posts)
	}
	unavailable = false
	for i := 0; i < 2; i++ {
		got, err := k.EnsureProjectNamespace(context.Background(), project)
		if err != nil || got != ns {
			t.Fatal(got, err)
		}
	}
	policy := objects["/apis/networking.k8s.io/v1/namespaces/"+ns+"/networkpolicies/application-isolation"]
	if rules := policy["spec"].(map[string]any)["egress"].([]any); len(rules) != 2 {
		t.Fatal("unexpected application egress", rules)
	}
	if posts != 6 {
		t.Fatal("retry duplicated resources", posts)
	}
	other := "prj_" + strings.Repeat("b", 32)
	got, err := k.EnsureProjectNamespace(context.Background(), other)
	if err != nil || got == ns || posts != 12 {
		t.Fatal("projects share a namespace", got, err, posts)
	}
	// A deleted namespace must not be silently adopted or recreated mid-deletion.
	objects["/api/v1/namespaces/"+ns]["metadata"].(map[string]any)["deletionTimestamp"] = "2026-11-15T00:00:00Z"
	if _, err = k.EnsureProjectNamespace(context.Background(), project); err == nil {
		t.Fatal("terminating namespace accepted")
	}
	foreign := "prj_" + strings.Repeat("c", 32)
	foreignNS := shared.ProjectNamespace(foreign)
	objects["/api/v1/namespaces/"+foreignNS] = map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": foreignNS, "labels": map[string]any{"owner": "another-application"}}}
	if _, err = k.EnsureProjectNamespace(context.Background(), foreign); err == nil || posts != 12 {
		t.Fatal("foreign namespace adopted", err)
	}
	for _, invalid := range []string{"default", "harbormaster", "kube-system", "../harbor", strings.Repeat("x", 128)} {
		if _, err = k.EnsureProjectNamespace(context.Background(), invalid); err == nil {
			t.Fatal("unsafe namespace admitted", invalid)
		}
	}
	// Owned resource drift is reported instead of overwriting deployment policy.
	quota := objects["/api/v1/namespaces/"+got+"/resourcequotas/application-quota"]
	quota["spec"].(map[string]any)["hard"].(map[string]any)["pods"] = "999"
	if _, err = k.EnsureProjectNamespace(context.Background(), other); err == nil {
		t.Fatal("quota drift ignored")
	}
}
