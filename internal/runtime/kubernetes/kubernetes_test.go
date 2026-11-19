package kubernetes

import (
	"agent-workbench/internal/runtime/protocol"
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-workbench/internal/shared"
)

type fakeKubernetes struct {
	mu                 sync.Mutex
	resources          map[string]map[string]any
	secrets            map[string]map[string]any
	creates            int
	requests           []string
	extraNetworkPolicy map[string]any
}

func (f *fakeKubernetes) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("Authorization") != "Bearer kube-service-token" {
		w.WriteHeader(401)
		return
	}
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 5 {
		w.WriteHeader(404)
		return
	}
	namespace := ""
	for i, v := range parts {
		if v == "namespaces" && i+1 < len(parts) {
			namespace = parts[i+1]
		}
	}
	key := namespace + "/" + parts[len(parts)-1]
	if strings.Contains(r.URL.Path, "/persistentvolumeclaims") {
		items := []any{}
		for key, b := range f.resources {
			if strings.HasPrefix(key, namespace+"/") {
				labels := b["metadata"].(map[string]any)["labels"]
				items = append(items, map[string]any{"metadata": map[string]any{"labels": labels}, "spec": map[string]any{"storageClassName": "test-rwop", "accessModes": []string{"ReadWriteOncePod"}}})
			}
		}
		shared.JSON(w, 200, map[string]any{"items": items})
		return
	}
	if strings.Contains(r.URL.Path, "/networkpolicies") {
		project := "proj_one"
		if namespace == "project-two" {
			project = "proj_two"
		}
		items := []any{}
		for name, spec := range projectPolicies(project, project, "harbormaster") {
			items = append(items, map[string]any{"metadata": map[string]string{"name": name}, "spec": spec})
		}
		if f.extraNetworkPolicy != nil {
			items = append(items, map[string]any{"metadata": map[string]string{"name": "additional-policy"}, "spec": f.extraNetworkPolicy})
		}
		shared.JSON(w, 200, map[string]any{"items": items})
		return
	}
	if strings.Contains(r.URL.Path, "/secrets") {
		if r.Method == "GET" {
			if s, ok := f.secrets[key]; ok {
				shared.JSON(w, 200, s)
			} else {
				w.WriteHeader(404)
			}
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		name := body["metadata"].(map[string]any)["name"].(string)
		f.secrets[namespace+"/"+name] = body
		shared.JSON(w, 201, body)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/sandboxes") {
		if r.Method == "GET" {
			items := []any{}
			selector := r.URL.Query().Get("labelSelector")
			for key, body := range f.resources {
				if !strings.HasPrefix(key, namespace+"/") {
					continue
				}
				labels := body["metadata"].(map[string]any)["labels"].(map[string]any)
				if selector != "" && !strings.Contains(selector, "workbench.workspace="+labels["workbench.workspace"].(string)) {
					continue
				}
				items = append(items, body)
			}
			shared.JSON(w, 200, map[string]any{"items": items})
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		meta := body["metadata"].(map[string]any)
		meta["generation"] = float64(1)
		meta["resourceVersion"] = "1"
		body["status"] = map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "True", "observedGeneration": float64(1)}}}
		f.resources[namespace+"/"+meta["name"].(string)] = body
		f.creates++
		shared.JSON(w, 201, body)
		return
	}
	if r.Method == "PATCH" {
		body, ok := f.resources[key]
		if !ok {
			w.WriteHeader(404)
			return
		}
		var patch map[string]any
		json.NewDecoder(r.Body).Decode(&patch)
		mode := patch["spec"].(map[string]any)["operatingMode"].(string)
		body["spec"].(map[string]any)["operatingMode"] = mode
		meta := body["metadata"].(map[string]any)
		generation := meta["generation"].(float64) + 1
		meta["generation"] = generation
		condition := "Ready"
		if mode == "Suspended" {
			condition = "Suspended"
		}
		body["status"] = map[string]any{"conditions": []any{map[string]any{"type": condition, "status": "True", "observedGeneration": generation}}}
		shared.JSON(w, 200, body)
		return
	}
	w.WriteHeader(404)
}

func kubeProvider(t *testing.T) (*Kubernetes, *fakeKubernetes) {
	t.Helper()
	fake := &fakeKubernetes{resources: map[string]map[string]any{}, secrets: map[string]map[string]any{}}
	server := httptest.NewTLSServer(http.HandlerFunc(fake.serve))
	t.Cleanup(server.Close)
	ca := filepath.Join(t.TempDir(), "kube-ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	c := KubernetesConfig{APIURL: server.URL, APIToken: "kube-service-token", CAFile: ca, Token: strings.Repeat("a", 32), GatewayServiceToken: strings.Repeat("b", 32), GrantSecret: strings.Repeat("c", 32), Deployment: "test", Image: "nexus.invalid/workspace@sha256:" + strings.Repeat("a", 64), RuntimeClass: "kata", StorageClass: "test-rwop", ServiceAccount: "workspace", InferenceURL: "http://inference:8080", BrokerURL: "http://broker:8080", ProjectNamespaces: map[string]string{"proj_one": "project-one", "proj_two": "project-two"}}
	k, err := NewKubernetes(c)
	if err != nil {
		t.Fatal(err)
	}
	k.agentHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.URL.Host, ".project-one.svc:8080") {
			t.Error("workspace route escaped project namespace")
		}
		body := `{"ready":true}`
		if r.URL.Path == "/v1/activity" {
			b, _ := json.Marshal(shared.Activity{Known: true, LastActivity: time.Now().UTC()})
			body = string(b)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	return k, fake
}

func TestKubernetesFixedSandboxOwnershipPersistenceAndObservedReadiness(t *testing.T) {
	k, fake := kubeProvider(t)
	k.cfg.CodexModel = "test-codex"
	k.cfg.ClaudeModel = "test-claude"
	ctx := context.Background()
	spec := protocol.WorkspaceSpec{ProjectID: "proj_one", HomeID: "home_one", AgentKind: "t3"}
	o, err := k.StartConfigured(ctx, "ws_test", spec)
	if err != nil || !o.Ready {
		t.Fatalf("start: %+v %v", o, err)
	}
	if o.IsolationStatus != "unverified" || o.NetworkStatus != "unverified" {
		t.Fatal("configuration was treated as security evidence")
	}
	body := fake.resources["project-one/test-ws-test"]
	pod := body["spec"].(map[string]any)["podTemplate"].(map[string]any)["spec"].(map[string]any)
	container := pod["containers"].([]any)[0].(map[string]any)
	models := map[string]string{}
	for _, entry := range container["env"].([]any) {
		v := entry.(map[string]any)
		if value, ok := v["value"].(string); ok {
			models[v["name"].(string)] = value
		}
	}
	if models["CODEX_MODEL"] != "test-codex" || models["CLAUDE_MODEL"] != "test-claude" {
		t.Fatal("provider model configuration missing")
	}
	if pod["runtimeClassName"] != "kata" || pod["automountServiceAccountToken"] != false || pod["serviceAccountName"] != "workspace" {
		t.Fatal("sandbox policy weakened")
	}
	for _, request := range fake.requests {
		if strings.Contains(request, "/pods") || strings.Contains(request, "/exec") {
			t.Fatal("custom Pod/exec API used instead of Agent Sandbox")
		}
	}
	if _, err = k.StartConfigured(ctx, "ws_test", protocol.WorkspaceSpec{ProjectID: "proj_two", HomeID: "home_one", AgentKind: "t3"}); err == nil {
		t.Fatal("workspace moved to another project")
	}
	if err = k.Stop(ctx, "ws_test"); err != nil {
		t.Fatal(err)
	}
	o, err = k.Inspect(ctx, "ws_test")
	if err != nil || o.Running {
		t.Fatal("observed suspension not retained")
	}
	if _, err = k.StartConfigured(ctx, "ws_test", spec); err != nil {
		t.Fatal(err)
	}
	if fake.creates != 1 || len(fake.secrets) != 1 {
		t.Fatal("resume duplicated sandbox identity or private state")
	}
	k.cfg.RequireVerified = true
	if _, err = k.StartConfigured(ctx, "ws_test", spec); err == nil {
		t.Fatal("secure gate bypassed")
	}
}

func TestKubernetesDeniesUnprovisionedProjectAndUnsupportedBranchBeforeCreation(t *testing.T) {
	k, fake := kubeProvider(t)
	for _, s := range []protocol.WorkspaceSpec{{ProjectID: "proj_missing", HomeID: "home_one"}, {ProjectID: "proj_one", HomeID: "home_one", ParentID: "ws_other"}, {ProjectID: "proj_one", HomeID: "home_one", AgentKind: "hermes"}} {
		if _, err := k.StartConfigured(context.Background(), "ws_test", s); err == nil {
			t.Fatal("unsupported claim accepted")
		}
	}
	if len(fake.requests) != 0 {
		t.Fatal("denied claim touched Kubernetes state")
	}
}

func TestProjectAllowlistPolicyPersistenceAndOwnership(t *testing.T) {
	k, f := kubeProvider(t)
	ctx := context.Background()
	k.cfg.ProjectAllowlists = map[string][]string{"proj_one": {"github.com:443"}}
	if _, err := k.StartConfigured(ctx, "ws_test", protocol.WorkspaceSpec{ProjectID: "proj_one", HomeID: "home_one", AgentKind: "t3"}); err != nil {
		t.Fatal(err)
	}
	if err := k.ApplyNetwork(ctx, "ws_test", shared.ConnectionPolicy{Mode: "allowlist", Destinations: []string{"evil.invalid:443"}}); err == nil {
		t.Fatal("project ceiling bypass")
	}
	if err := k.ApplyNetwork(ctx, "ws_test", shared.ConnectionPolicy{Mode: "open"}); err == nil {
		t.Fatal("open accepted")
	}
	p := shared.ConnectionPolicy{Mode: "allowlist", Destinations: []string{"github.com:443"}}
	if err := k.ApplyNetwork(ctx, "ws_test", p); err != nil {
		t.Fatal(err)
	}
	r, err := k.find(ctx, "ws_test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := k.readPolicy(ctx, r)
	if err != nil || !got.Allows("github.com:443") {
		t.Fatalf("policy lost: %+v %v", got, err)
	}
	f.secrets["project-one/test-ws-test-network"]["metadata"].(map[string]any)["labels"].(map[string]any)["workbench.home"] = "home_other"
	if _, err = k.readPolicy(ctx, r); err == nil {
		t.Fatal("foreign policy accepted")
	}
}

func TestNetworkEvidenceRejectsAdditivePolicyAndAdmissionFailure(t *testing.T) {
	for _, status := range []int{403, 409, 422, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			k, _ := kubeProvider(t)
			k.apiHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
			})}
			if _, err := k.StartConfigured(context.Background(), "ws_test", protocol.WorkspaceSpec{ProjectID: "proj_one", HomeID: "home_one"}); err == nil {
				t.Fatal("admission failure ignored")
			}
		})
	}
	k, _ := kubeProvider(t)
	k.apiHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"items":[{"metadata":{"name":"allow-everything"},"spec":{}}]}`)), Header: http.Header{}}, nil
	})}
	if err := k.networkEvidence(context.Background(), "project-one", "proj_one"); err == nil {
		t.Fatal("additive policy accepted")
	}
}

func TestSandboxQuotaAndAdmissionDenialsDoNotCreateWorkspace(t *testing.T) {
	for _, code := range []int{403, 422, 429} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			k, f := kubeProvider(t)
			original := k.apiHTTP.Transport
			k.apiHTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/sandboxes") {
					return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				}
				return original.RoundTrip(r)
			})
			if _, err := k.StartConfigured(context.Background(), "ws_test", protocol.WorkspaceSpec{ProjectID: "proj_one", HomeID: "home_one"}); err == nil {
				t.Fatal("denial ignored")
			}
			if f.creates != 0 {
				t.Fatal("workspace created after denial")
			}
		})
	}
}
func TestStorageEvidenceRejectsForeignHomeAndWeakenedFence(t *testing.T) {
	k, _ := kubeProvider(t)
	var r kubeResource
	r.Metadata.Namespace = "project-one"
	r.Metadata.Labels = map[string]string{"workbench.workspace": "ws_test", "workbench.project": "proj_one", "workbench.home": "home_one", "workbench.deployment": "test", "workbench.generation": "gen_one"}
	for _, home := range []string{"home_other", "home_one"} {
		labels := map[string]string{}
		for a, b := range r.Metadata.Labels {
			labels[a] = b
		}
		labels["workbench.home"] = home
		mode := "ReadWriteOncePod"
		if home == "home_one" {
			mode = "ReadWriteOnce"
		}
		b, _ := json.Marshal(map[string]any{"items": []any{map[string]any{"metadata": map[string]any{"labels": labels}, "spec": map[string]any{"storageClassName": "test-rwop", "accessModes": []string{mode}}}}})
		k.apiHTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
		})}
		if err := k.storageEvidence(context.Background(), r); err == nil {
			t.Fatal("storage ownership/fence bypass")
		}
	}
}
