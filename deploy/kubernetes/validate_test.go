package kubernetes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type object map[string]any

func field(o any, keys ...string) any {
	for _, k := range keys {
		m, ok := o.(map[string]any)
		if !ok {
			return nil
		}
		o = m[k]
	}
	return o
}
func list(o any) []any { v, _ := o.([]any); return v }
func build(t *testing.T, path string) []object {
	t.Helper()
	binary := os.Getenv("KUSTOMIZE_BINARY")
	args := []string{"build", path}
	if binary == "" {
		t.Skip("run make kubernetes-check to use the pinned Kustomize tool")
	}
	out, err := exec.Command(binary, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("kustomize %s: %v\n%s", path, err, out)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(out))
	var objects []object
	seen := map[string]bool{}
	for {
		var o map[string]any
		if err := decoder.Decode(&o); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if len(o) == 0 {
			continue
		}
		key := key(o)
		if seen[key] {
			t.Fatalf("duplicate resource %s", key)
		}
		seen[key] = true
		objects = append(objects, o)
	}
	return objects
}
func key(o map[string]any) string {
	b, _ := json.Marshal([]any{o["apiVersion"], o["kind"], field(o, "metadata", "namespace"), field(o, "metadata", "name")})
	return string(b)
}
func find(t *testing.T, objects []object, kind, name string) object {
	t.Helper()
	for _, o := range objects {
		if o["kind"] == kind && field(map[string]any(o), "metadata", "name") == name {
			return o
		}
	}
	t.Fatalf("missing %s/%s", kind, name)
	return nil
}
func env(o object) map[string]string {
	result := map[string]string{}
	containers := list(field(map[string]any(o), "spec", "template", "spec", "containers"))
	for _, c := range containers {
		for _, e := range list(field(c, "env")) {
			name, _ := field(e, "name").(string)
			value, _ := field(e, "value").(string)
			result[name] = value
		}
	}
	return result
}

func TestPlatformSecurityAndExampleContract(t *testing.T) {
	digest := regexp.MustCompile(`^.+@sha256:[a-f0-9]{64}$`)
	for _, path := range []string{"base", "overlays/example"} {
		t.Run(path, func(t *testing.T) {
			objects := build(t, path)
			deployments := 0
			for _, o := range objects {
				raw := map[string]any(o)
				switch o["kind"] {
				case "Secret", "Sandbox", "Job", "DaemonSet", "RuntimeClass":
					t.Fatalf("%s must not own credentials, workspaces or host runtime: %s", path, key(raw))
				case "Namespace", "StorageClass", "Cluster", "CustomResourceDefinition":
					options, _ := field(raw, "metadata", "annotations", "argocd.argoproj.io/sync-options").(string)
					if !strings.Contains(options, "Prune=false") || !strings.Contains(options, "Delete=false") {
						t.Fatalf("state protection missing: %s", key(raw))
					}
				case "Deployment":
					deployments++
					name := field(raw, "metadata", "name")
					pod := field(raw, "spec", "template", "spec")
					if field(pod, "automountServiceAccountToken") != (name == "runtime-controller" || name == "agent-sandbox-controller") {
						t.Fatalf("unexpected Kubernetes token access: %v", name)
					}
					if field(pod, "securityContext", "runAsNonRoot") != true {
						t.Fatalf("root pod: %v", name)
					}
					for _, v := range list(field(pod, "volumes")) {
						if field(v, "hostPath") != nil {
							t.Fatalf("host mount: %v", name)
						}
					}
					for _, c := range list(field(pod, "containers")) {
						image, _ := field(c, "image").(string)
						if !digest.MatchString(image) && image != "ghcr.io/robrotheram/harbormaster/"+fmt.Sprint(name) {
							t.Fatalf("invalid image reference: %s", image)
						}
						if field(c, "securityContext", "allowPrivilegeEscalation") != false || field(c, "securityContext", "readOnlyRootFilesystem") != true {
							t.Fatalf("unrestricted container: %v", name)
						}
					}
				case "Service":
					kind := field(raw, "spec", "type")
					if kind != nil && kind != "ClusterIP" && kind != "ExternalName" {
						t.Fatalf("public service: %s", key(raw))
					}
				}
			}
			if deployments != 8 {
				t.Fatalf("expected 8 platform deployments, got %d", deployments)
			}
			values := env(find(t, objects, "Deployment", "runtime-controller"))
			if values["RUNTIME_BACKEND"] != "kubernetes" || values["KUBERNETES_DISCOVER_GATEWAYS"] != "true" {
				t.Fatal("invalid runtime configuration")
			}
			if values["REQUIRE_VERIFIED_ISOLATION"] != "true" || values["WORKSPACE_IMAGE"] != "ghcr.io/robrotheram/harbormaster/workspace" {
				t.Fatal("deployment source must fail closed until configured with a pinned workspace image")
			}
			if path == "base" {
				for _, o := range objects {
					if o["kind"] == "Ingress" {
						t.Fatal("base must not expose an unauthenticated dashboard")
					}
				}
				return
			}
			dashboard := find(t, objects, "Deployment", "dashboard")
			origin := env(dashboard)["DASHBOARD_PUBLIC_ORIGIN"]
			ingress := find(t, objects, "Ingress", "workbench")
			rules := list(field(map[string]any(ingress), "spec", "rules"))
			if len(rules) != 1 || "https://"+fmt.Sprint(field(rules[0], "host")) != origin || env(dashboard)["WORKOS_REDIRECT_URI"] != origin+"/auth/callback" {
				t.Fatal("example HTTPS origin, ingress and authentication callback must agree")
			}
			pod := field(map[string]any(dashboard), "spec", "template", "spec")
			authEnv := map[string]bool{}
			for _, container := range list(field(pod, "containers")) {
				for _, entry := range list(field(container, "env")) {
					if field(entry, "valueFrom", "secretKeyRef", "name") == "dashboard-secrets" {
						authEnv[fmt.Sprint(field(entry, "name"))] = true
					}
				}
			}
			if !authEnv["WORKOS_CLIENT_ID"] || !authEnv["WORKOS_API_KEY"] || !authEnv["WORKOS_ALLOWED_EMAILS"] {
				t.Fatal("example dashboard must require its WorkOS configuration")
			}
			role := find(t, objects, "Role", "workbench-runtime")
			for _, rule := range list(role["rules"]) {
				for _, resource := range list(field(rule, "resources")) {
					if resource == "pods" || resource == "pods/exec" || resource == "*" {
						t.Fatal("runtime role grants direct pod access")
					}
				}
			}
			binding := find(t, objects, "RoleBinding", "workbench-runtime")
			if field(list(binding["subjects"])[0], "namespace") != "harbormaster" {
				t.Fatal("runtime service account binding was rewritten")
			}
		})
	}
}

func TestDeploymentContainsNoScripts(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".py", ".sh", ".bash", ".ps1":
			t.Errorf("deployment script remains: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHermesIsAnOptInComponent(t *testing.T) {
	directory := t.TempDir()
	base, err := filepath.Abs("base")
	if err != nil {
		t.Fatal(err)
	}
	component, err := filepath.Abs("components/hermes")
	if err != nil {
		t.Fatal(err)
	}
	base, err = filepath.Rel(directory, base)
	if err != nil {
		t.Fatal(err)
	}
	component, err = filepath.Rel(directory, component)
	if err != nil {
		t.Fatal(err)
	}
	spec := map[string]any{"apiVersion": "kustomize.config.k8s.io/v1beta1", "kind": "Kustomization", "resources": []string{base}, "components": []string{component}}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "kustomization.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	objects := build(t, directory)
	hermes := find(t, objects, "Deployment", "hermes-mcp")
	if field(map[string]any(hermes), "spec", "template", "spec", "automountServiceAccountToken") != false {
		t.Fatal("Hermes MCP must not receive a Kubernetes token")
	}
	if env(find(t, objects, "Deployment", "runtime-controller"))["REQUIRE_VERIFIED_ISOLATION"] != "true" {
		t.Fatal("optional component must not open workspace execution")
	}
}

func TestConsolidatedCoreRetainsWorkspaceBoundary(t *testing.T) {
	for _, path := range []string{"base", "overlays/example"} {
		t.Run(path, func(t *testing.T) {
			objects := build(t, path)
			namespaces := map[string]bool{}
			for _, o := range objects {
				raw := map[string]any(o)
				name, _ := field(raw, "metadata", "name").(string)
				ns, _ := field(raw, "metadata", "namespace").(string)
				if o["kind"] == "Namespace" {
					namespaces[name] = true
				}
				switch o["kind"] {
				case "Deployment", "Service", "Cluster", "Certificate", "Middleware", "Ingress", "ConfigMap", "ServiceAccount":
					if ns != "harbormaster" {
						t.Fatalf("core resource outside consolidated namespace: %s", key(raw))
					}
				}
				if o["kind"] == "Service" && field(raw, "spec", "type") == "ExternalName" {
					t.Fatal("obsolete cross-namespace service alias")
				}
			}
			expected := map[string]bool{"harbormaster": true}
			if !reflect.DeepEqual(namespaces, expected) {
				t.Fatalf("unexpected namespace layout: %v", namespaces)
			}
			if path == "base" {
				return
			}
			for _, policyName := range []string{"workspace-controller-ingress", "workspace-project-egress"} {
				policy := map[string]any(find(t, objects, "NetworkPolicy", policyName))
				if field(policy, "metadata", "namespace") != "harbormaster" {
					t.Fatal("workspace policy must share the core namespace")
				}
				direction, peers := "egress", "to"
				if policyName == "workspace-controller-ingress" {
					direction, peers = "ingress", "from"
				}
				for _, rule := range list(field(policy, "spec", direction)) {
					for _, peer := range list(field(rule, peers)) {
						if field(peer, "namespaceSelector", "matchLabels", "kubernetes.io/metadata.name") != "harbormaster" {
							t.Fatal("wrong core namespace selector")
						}
						labels, ok := field(peer, "podSelector", "matchLabels").(map[string]any)
						if !ok || len(labels) == 0 {
							t.Fatal("workspace can reach entire core namespace")
						}
						if direction == "egress" && (labels["workbench.project"] != "default" || labels["workbench.role"] == nil) {
							t.Fatal("gateway project/role restrictions lost")
						}
					}
				}
			}
		})
	}
}

func TestExampleUsesIsolatedProjectWorkers(t *testing.T) {
	objects := build(t, "overlays/example")
	if env(find(t, objects, "Deployment", "controlplane"))["SHARED_WORKSPACE_ID"] != "" {
		t.Fatal("shared worker must be disabled")
	}
	if env(find(t, objects, "Deployment", "runtime-controller"))["PROVISION_WORKSPACE_NAMESPACES"] != "true" {
		t.Fatal("isolated worker provisioning not configured")
	}
	quota := map[string]any(find(t, objects, "ResourceQuota", "project-quota"))
	if field(quota, "metadata", "namespace") != "harbormaster" || field(quota, "spec", "hard", "pods") != "1" {
		t.Fatal("legacy namespace must retain its one-worker limit")
	}
}

func TestSharedNamespaceKeepsWorkspacePoliciesOffCoreServices(t *testing.T) {
	objects := build(t, "overlays/example")
	for _, name := range []string{"workspace-default-deny", "workspace-controller-ingress", "workspace-project-egress"} {
		policy := find(t, objects, "NetworkPolicy", name)
		expressions := list(field(map[string]any(policy), "spec", "podSelector", "matchExpressions"))
		if len(expressions) != 1 || field(expressions[0], "key") != "workbench.workspace" || field(expressions[0], "operator") != "Exists" {
			t.Fatal("workspace policy selects platform services", name)
		}
	}
	quota := find(t, objects, "ResourceQuota", "project-quota")
	scopes := list(field(map[string]any(quota), "spec", "scopeSelector", "matchExpressions"))
	if len(scopes) != 1 || field(scopes[0], "scopeName") != "PriorityClass" || !reflect.DeepEqual(field(scopes[0], "values"), []any{"harbormaster-workspace"}) {
		t.Fatal("workspace quota applies to core services")
	}
	binding := find(t, objects, "ClusterRoleBinding", "agent-sandbox-controller")
	for _, subject := range list(binding["subjects"]) {
		if field(subject, "namespace") != "harbormaster" {
			t.Fatal("sandbox controller binding outside core")
		}
	}
	if env(find(t, objects, "Deployment", "controlplane"))["PROVISION_PROJECT_NAMESPACES"] != "true" || env(find(t, objects, "Deployment", "runtime-controller"))["PROVISION_PROJECT_NAMESPACES"] != "true" {
		t.Fatal("automatic project namespaces not enabled")
	}
}
