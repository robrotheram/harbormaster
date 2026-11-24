package kubernetes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkspaceNamespacesArePrivateRetainedAndRediscovered(t *testing.T) {
	k, _ := kubeProvider(t)
	k.cfg.ProvisionWorkspaceNamespaces = true
	k.cfg.SharedGatewayProject = "default"
	objects := map[string]map[string]any{}
	posts := 0
	sandboxItems := []any{}
	var requests []string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status := 200
		var body any
		path := r.URL.Path
		requests = append(requests, r.URL.RequestURI())
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
					name := o["metadata"].(map[string]any)["name"].(string)
					if field := r.URL.Query().Get("fieldSelector"); field == "" || field == "metadata.name="+name {
						items = append(items, o)
					}
				}
			}
			body = map[string]any{"items": items}
		case strings.HasSuffix(path, "/sandboxes") && r.Method == "GET":
			items := []any{}
			for _, v := range sandboxItems {
				o := v.(map[string]any)
				ns := o["metadata"].(map[string]any)["namespace"].(string)
				if path == k.collection(ns) {
					items = append(items, o)
				}
			}
			body = map[string]any{"items": items}
		case strings.HasSuffix(path, "/networkpolicies") && r.Method == "GET":
			items := []any{}
			for key, o := range objects {
				if strings.HasPrefix(key, path+"/") {
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
	labels := map[string]any{"workbench.deployment": k.cfg.Deployment, "workbench.workspace": "ws_private", "workbench.project": projects[0], "workbench.generation": "gen_private", "workbench.home": "home_private"}
	sandboxItems = append(sandboxItems, map[string]any{"metadata": map[string]any{"name": k.name("ws_private"), "namespace": workspaceNamespace(projects[0]), "labels": labels, "generation": 1}, "spec": map[string]any{"operatingMode": "Running"}, "status": map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "True", "observedGeneration": 1}}}})
	ns := workspaceNamespace(projects[0])
	if _, err := restarted.find(ctx, "ws_private"); err != nil {
		t.Fatal("cold workspace discovery", err)
	}
	requests = nil
	if _, err := restarted.find(ctx, "ws_private"); err != nil {
		t.Fatal("warm workspace lookup", err)
	}
	if len(requests) != 3 || !strings.HasPrefix(requests[0], k.collection(ns)+"?") || !strings.Contains(requests[1], "fieldSelector=metadata.name%3D"+ns) || !strings.HasSuffix(requests[2], "/namespaces/"+ns+"/rolebindings/workbench-runtime") {
		t.Fatalf("warm workspace lookup searched unrelated namespaces: %v", requests)
	}
	verify := func() *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]string{"token": k.token("ws_private", "gen_private"), "audience": "inference"})
		r := httptest.NewRequest("POST", "/v1/grants/verify", strings.NewReader(string(b)))
		w := httptest.NewRecorder()
		restarted.verify(w, r)
		return w
	}
	if got := verify(); got.Code != 200 || !strings.Contains(got.Body.String(), projects[0]) {
		t.Fatal("dynamic workspace grant rejected after restart", got.Code, got.Body)
	}
	requests = nil
	if got := verify(); got.Code != 200 {
		t.Fatal("warm grant rejected", got.Code, got.Body)
	}
	if len(requests) != 4 || !strings.HasPrefix(requests[0], k.collection(ns)+"?") || !strings.Contains(requests[1], "fieldSelector=metadata.name%3D"+ns) {
		t.Fatalf("warm grant looked outside the workspace namespace: %v", requests)
	}
	labels["workbench.generation"] = "gen_replacement"
	if got := verify(); got.Code != 403 {
		t.Fatal("cached grant authorized a different generation", got.Code, got.Body)
	}
	labels["workbench.generation"] = "gen_private"
	labels["workbench.project"] = projects[1]
	if got := verify(); got.Code != 403 {
		t.Fatal("cross-project grant accepted", got.Code, got.Body)
	}
	labels["workbench.project"] = projects[0]
	delete(labels, "workbench.home")
	if got := verify(); got.Code != 403 {
		t.Fatal("grant accepted without home ownership", got.Code, got.Body)
	}
	labels["workbench.home"] = "home_private"
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
	objects["/api/v1/namespaces/"+ns]["metadata"].(map[string]any)["labels"].(map[string]any)["workbench.project"] = projects[1]
	if _, err := restarted.find(ctx, "ws_private"); err == nil {
		t.Fatal("cached route accepted namespace ownership drift")
	}
	if err := k.ensureWorkspaceNamespace(ctx, projects[0]); err == nil {
		t.Fatal("foreign namespace adopted")
	}
	if _, err := restarted.runtimeNamespaces(ctx); err == nil {
		t.Fatal("foreign namespace discovered")
	}
}
