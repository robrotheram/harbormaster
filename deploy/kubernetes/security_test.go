package kubernetes_test

import "testing"

func TestCoreIngressDoesNotChangeWorkspacePolicy(t *testing.T) {
	for _, path := range []string{"base", "overlays/example"} {
		t.Run(path, func(t *testing.T) {
			objects := build(t, path)
			namespace := find(t, objects, "Namespace", "harbormaster")
			if field(map[string]any(namespace), "metadata", "labels", "pod-security.kubernetes.io/enforce") != "restricted" {
				t.Fatal("core namespace must enforce restricted Pod Security")
			}
			deny := map[string]any(find(t, objects, "NetworkPolicy", "core-default-deny"))
			if len(list(field(deny, "spec", "ingress"))) != 0 {
				t.Fatal("core default-deny policy permits ingress")
			}
			for _, obj := range objects {
				raw := map[string]any(obj)
				switch obj["kind"] {
				case "Deployment":
					for _, container := range list(field(raw, "spec", "template", "spec", "containers")) {
						if field(container, "livenessProbe") == nil {
							t.Fatalf("deployment has no liveness probe: %s", key(raw))
						}
					}
				case "NetworkPolicy":
					name := field(raw, "metadata", "name")
					switch name {
					case "workspace-default-deny", "workspace-controller-ingress", "workspace-project-egress":
						continue
					}
					excludesWorkspaces := false
					for _, requirement := range list(field(raw, "spec", "podSelector", "matchExpressions")) {
						if field(requirement, "key") == "workbench.workspace" && field(requirement, "operator") == "DoesNotExist" {
							excludesWorkspaces = true
							break
						}
					}
					if !excludesWorkspaces {
						t.Fatalf("core policy may select workspace Pods: %s", key(raw))
					}
				}
			}
		})
	}
}
