package kubernetes

import (
	"context"
	"errors"
	"reflect"

	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
)

// EnsureProjectNamespace provisions application destinations, never agent
// workspaces. It cannot adopt an unrelated namespace or delete deployed data.
func (k *Kubernetes) EnsureProjectNamespace(ctx context.Context, project string) (string, error) {
	namespace := shared.ProjectNamespace(project)
	if !k.cfg.ProvisionProjectNamespaces || namespace == "" || namespace == k.cfg.CoreNamespace {
		return "", errors.New("project namespace provisioning unavailable")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	labels := map[string]any{"app.kubernetes.io/managed-by": "harbormaster", "harbormaster.io/deployment": k.cfg.Deployment, "harbormaster.io/project": project, "harbormaster.io/purpose": "application"}
	object := func(api, kind, name string) map[string]any {
		return map[string]any{"apiVersion": api, "kind": kind, "metadata": map[string]any{"name": name, "namespace": namespace, "labels": labels}}
	}
	ns := object("v1", "Namespace", namespace)
	delete(ns["metadata"].(map[string]any), "namespace")
	nsLabels := map[string]any{}
	for key, value := range labels {
		nsLabels[key] = value
	}
	nsLabels["pod-security.kubernetes.io/enforce"] = "restricted"
	nsLabels["pod-security.kubernetes.io/enforce-version"] = "latest"
	ns["metadata"].(map[string]any)["labels"] = nsLabels
	ns["metadata"].(map[string]any)["annotations"] = map[string]any{"argocd.argoproj.io/sync-options": "Prune=false,Delete=false"}
	if err := k.ensureProjectObject(ctx, "/api/v1/namespaces", ns); err != nil {
		return "", err
	}
	base := "/api/v1/namespaces/" + namespace
	limits := object("v1", "LimitRange", "application-defaults")
	limits["spec"] = map[string]any{"limits": []any{map[string]any{"type": "Container", "defaultRequest": map[string]any{"cpu": "100m", "memory": "128Mi"}, "default": map[string]any{"cpu": "1", "memory": "1Gi"}}}}
	quota := object("v1", "ResourceQuota", "application-quota")
	quota["spec"] = map[string]any{"hard": map[string]any{"pods": "10", "requests.cpu": "2", "limits.cpu": "4", "requests.memory": "4Gi", "limits.memory": "8Gi", "requests.storage": "20Gi", "persistentvolumeclaims": "10", "services.loadbalancers": "0", "services.nodeports": "0"}}
	policy := object("networking.k8s.io/v1", "NetworkPolicy", "application-isolation")
	sameNamespace := map[string]any{"podSelector": map[string]any{}}
	dns := map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]any{"kubernetes.io/metadata.name": "kube-system"}}, "podSelector": map[string]any{"matchLabels": map[string]any{"k8s-app": "kube-dns"}}}
	ingress := map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]any{"kubernetes.io/metadata.name": "kube-system"}}, "podSelector": map[string]any{"matchLabels": map[string]any{"app.kubernetes.io/name": "traefik"}}}
	policy["spec"] = map[string]any{"podSelector": map[string]any{}, "policyTypes": []any{"Ingress", "Egress"}, "ingress": []any{map[string]any{"from": []any{sameNamespace, ingress}}}, "egress": []any{
		map[string]any{"to": []any{sameNamespace}},
		map[string]any{"to": []any{dns}, "ports": []any{map[string]any{"protocol": "UDP", "port": float64(53)}, map[string]any{"protocol": "TCP", "port": float64(53)}}},
		map[string]any{"to": []any{map[string]any{"ipBlock": map[string]any{"cidr": "0.0.0.0/0", "except": []any{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "169.254.0.0/16"}}}}},
	}}
	for _, entry := range []struct {
		path string
		body map[string]any
	}{{base + "/limitranges", limits}, {base + "/resourcequotas", quota}, {"/apis/networking.k8s.io/v1/namespaces/" + namespace + "/networkpolicies", policy}} {
		if err := k.ensureProjectObject(ctx, entry.path, entry.body); err != nil {
			return "", err
		}
	}
	// Registry credentials are copied only from the configured core namespace;
	// the runtime endpoint never accepts secret names or namespace names as input.
	var registry map[string]any
	if err := k.call(ctx, "GET", "/api/v1/namespaces/"+k.cfg.CoreNamespace+"/secrets/harbormaster-registry", nil, &registry); err != nil {
		return "", err
	}
	data, ok := registry["data"].(map[string]any)
	if !ok || registry["type"] != "kubernetes.io/dockerconfigjson" || data[".dockerconfigjson"] == nil {
		return "", errors.New("registry credentials unavailable")
	}
	secret := object("v1", "Secret", "harbormaster-registry")
	secret["type"] = registry["type"]
	secret["data"] = data
	if err := k.ensureProjectObject(ctx, base+"/secrets", secret); err != nil {
		return "", err
	}
	account := object("v1", "ServiceAccount", "application")
	account["automountServiceAccountToken"] = false
	account["imagePullSecrets"] = []any{map[string]any{"name": "harbormaster-registry"}}
	if err := k.ensureProjectObject(ctx, base+"/serviceaccounts", account); err != nil {
		return "", err
	}
	return namespace, nil
}

func (k *Kubernetes) ensureProjectObject(ctx context.Context, collection string, want map[string]any) error {
	name := want["metadata"].(map[string]any)["name"].(string)
	var got map[string]any
	err := k.call(ctx, "GET", collection+"/"+name, nil, &got)
	if isNotFound(err) {
		err = k.call(ctx, "POST", collection, want, nil)
		if err == nil {
			return nil
		}
		var status protocol.APIError
		if !errors.As(err, &status) || status.Status != 409 {
			return err
		}
		err = k.call(ctx, "GET", collection+"/"+name, nil, &got)
	}
	if err != nil {
		return err
	}
	meta, ok := got["metadata"].(map[string]any)
	if !ok || meta["deletionTimestamp"] != nil || !containsDesired(got, want) {
		return errors.New("project namespace ownership or configuration conflict")
	}
	return nil
}

// Kubernetes adds defaults. Require our owned fields to match without rejecting
// unrelated defaulted fields or silently overwriting an operator's changes.
func containsDesired(got, want any) bool {
	switch value := want.(type) {
	case map[string]any:
		actual, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for key, v := range value {
			if !containsDesired(actual[key], v) {
				return false
			}
		}
		return true
	case []any:
		actual, ok := got.([]any)
		if !ok || len(actual) != len(value) {
			return false
		}
		for i, v := range value {
			if !containsDesired(actual[i], v) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(got, want)
	}
}
