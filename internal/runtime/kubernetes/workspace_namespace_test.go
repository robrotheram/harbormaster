package kubernetes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWorkspaceNamespacesArePrivateRetainedAndRediscovered(t *testing.T) {
	k, _ := kubeProvider(t)
	k.cfg.ProvisionWorkspaceNamespaces = true
	k.cfg.SharedGatewayProject = "default"
	objects := map[string]map[string]any{}
	posts := 0
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status := 200
		var body any
		path := r.URL.Path
		switch {
		case path == "/api/v1/namespaces/harbormaster/secrets/harbormaster-registry":
			body = map[string]any{"type": "kubernetes.io/dockerconfigjson", "data": map[string]any{".dockerconfigjson": "e30="}}
		case path == "/api/v1/namespaces" && r.Method == "GET":
			if !strings.Contains(r.URL.Query().Get("labelSelector"), "harbormaster.io/purpose=workspace") {
				t.Fatal("unscoped namespace discovery")
			}
			items := []any{}
			for _, o := range objects {
				if o["kind"] == "Namespace" {
					items = append(items, o)
				}
			}
			body = map[string]any{"items": items}
		case r.Method == "GET":
			var ok bool
			body, ok = objects[path]
			if !ok {
				status = 404
			}
		case r.Method == "POST":
			var o map[string]any
			if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
				t.Fatal(err)
			}
			if o["kind"] == "Pod" || o["kind"] == "Sandbox" {
				t.Fatal("provisioning created compute")
			}
			key := path + "/" + o["metadata"].(map[string]any)["name"].(string)
			if objects[key] != nil {
				status = 409
			} else {
				objects[key] = o
				posts++
				status = 201
			}
			body = o
		default:
			t.Fatalf("unexpected mutation %s %s", r.Method, path)
		}
		b, _ := json.Marshal(body)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})
	k.apiHTTP = &http.Client{Transport: transport}
	ctx := context.Background()
	projects := []string{"prj_" + strings.Repeat("a", 32), "prj_" + strings.Repeat("b", 32)}
	for _, p := range projects {
		for i := 0; i < 2; i++ {
			if err := k.ensureWorkspaceNamespace(ctx, p); err != nil {
				t.Fatal(err)
			}
		}
		ns := workspaceNamespace(p)
		if ns == "" || ns == k.cfg.CoreNamespace || strings.HasPrefix(ns, "hm-prj-") {
			t.Fatal("unsafe namespace", ns)
		}
		binding := objects["/apis/rbac.authorization.k8s.io/v1/namespaces/"+ns+"/rolebindings/workbench-runtime"]
		if binding["roleRef"].(map[string]any)["name"] != "harbormaster-workspace-runtime" {
			t.Fatal("unexpected role")
		}
		account := objects["/api/v1/namespaces/"+ns+"/serviceaccounts/"+k.cfg.ServiceAccount]
		if account["automountServiceAccountToken"] != false {
			t.Fatal("worker gets API credentials")
		}
		policy := objects["/apis/networking.k8s.io/v1/namespaces/"+ns+"/networkpolicies/workspace-project-egress"]["spec"].(map[string]any)
		if policy["podSelector"].(map[string]any)["matchLabels"].(map[string]any)["workbench.project"] != p {
			t.Fatal("shared worker selector")
		}
		for _, rule := range policy["egress"].([]any) {
			for _, v := range rule.(map[string]any)["to"].([]any) {
				peer := v.(map[string]any)
				labels := peer["podSelector"].(map[string]any)["matchLabels"].(map[string]any)
				if labels["workbench.project"] != "default" || labels["workbench.role"] == nil || peer["namespaceSelector"] == nil {
					t.Fatal("unrestricted shared peer")
				}
			}
		}
	}
	if posts != 16 {
		t.Fatal("not idempotent", posts)
	}
	restarted := &Kubernetes{cfg: k.cfg, apiHTTP: k.apiHTTP}
	namespaces, err := restarted.runtimeNamespaces(ctx)
	if err != nil || len(namespaces) != len(k.cfg.ProjectNamespaces)+2 {
		t.Fatal("restart lost namespaces", namespaces, err)
	}
	partialPath := "/apis/rbac.authorization.k8s.io/v1/namespaces/" + workspaceNamespace(projects[1]) + "/rolebindings/workbench-runtime"
	binding := objects[partialPath]
	delete(objects, partialPath)
	if discovered, err := restarted.runtimeNamespaces(ctx); err != nil || len(discovered) != len(namespaces)-1 {
		t.Fatal("partial provisioning blocked healthy namespaces", discovered, err)
	}
	objects[partialPath] = binding
	for _, id := range []string{"default", "kube-system", "prj_bad", "../default"} {
		if err := k.ensureWorkspaceNamespace(ctx, id); err == nil {
			t.Fatal("unsafe project accepted", id)
		}
	}
	ns := workspaceNamespace(projects[0])
	objects["/api/v1/namespaces/"+ns]["metadata"].(map[string]any)["labels"].(map[string]any)["workbench.project"] = projects[1]
	if err := k.ensureWorkspaceNamespace(ctx, projects[0]); err == nil {
		t.Fatal("foreign namespace adopted")
	}
	if _, err := restarted.runtimeNamespaces(ctx); err == nil {
		t.Fatal("foreign namespace discovered")
	}
}
