package kubernetes

import (
	"context"
	"testing"
)

func TestCoreNamespacePeersRemainPodScoped(t *testing.T) {
	policies := projectPolicies("project-a", "project-a", "platform")
	if rules := policies["workspace-project-egress"]["egress"].([]any); len(rules) != 1 {
		t.Fatalf("expected one workspace egress rule, got %d", len(rules))
	}
	for _, direction := range []string{"ingress", "egress"} {
		name, peers := "workspace-controller-ingress", "from"
		if direction == "egress" {
			name, peers = "workspace-project-egress", "to"
		}
		for _, entry := range policies[name][direction].([]any) {
			for _, value := range entry.(map[string]any)[peers].([]any) {
				peer := value.(map[string]any)
				ns := peer["namespaceSelector"].(map[string]any)["matchLabels"].(map[string]any)
				labels := peer["podSelector"].(map[string]any)["matchLabels"].(map[string]any)
				if ns["kubernetes.io/metadata.name"] != "platform" {
					t.Fatalf("wrong namespace: %v", ns)
				}
				if direction == "ingress" && labels["app"] != "runtime-controller" {
					t.Fatal("ingress permits other core services")
				}
				if direction == "egress" && (labels["workbench.project"] != "project-a" || labels["workbench.role"] != "gateway") {
					t.Fatal("egress lost project/role boundary")
				}
			}
		}
	}
}

func TestNetworkEvidenceAllowsPoliciesThatExcludeWorkspaces(t *testing.T) {
	k, fake := kubeProvider(t)
	spec := map[string]any{
		"podSelector": map[string]any{
			"matchExpressions": []any{map[string]string{"key": "workbench.workspace", "operator": "DoesNotExist"}},
		},
		"policyTypes": []string{"Ingress"},
	}
	fake.extraNetworkPolicy = spec
	if err := k.networkEvidence(context.Background(), "project-one", "proj_one"); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	spec["podSelector"] = map[string]any{}
	fake.mu.Unlock()
	if err := k.networkEvidence(context.Background(), "project-one", "proj_one"); err == nil {
		t.Fatal("unrecognized policy selecting workspace Pods passed validation")
	}
}
