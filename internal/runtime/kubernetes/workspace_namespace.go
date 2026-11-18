package kubernetes

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"

	"agent-workbench/internal/shared"
)

func workspaceNamespace(project string) string {
	ns := shared.ProjectNamespace(project)
	if ns == "" {
		return ""
	}
	return strings.Replace(ns, "hm-prj-", "hm-wrk-", 1)
}
func (k *Kubernetes) workspaceNamespace(project string) string {
	if ns := k.cfg.ProjectNamespaces[project]; ns != "" {
		return ns
	}
	if k.cfg.ProvisionWorkspaceNamespaces {
		return workspaceNamespace(project)
	}
	return ""
}

// Discover owned namespaces on every lookup so restart and concurrent project
// creation never lose bindings or require a mutable, process-local tenant map.
func (k *Kubernetes) runtimeNamespaces(ctx context.Context) ([]string, error) {
	namespaces := k.namespaces()
	if !k.cfg.ProvisionWorkspaceNamespaces {
		return namespaces, nil
	}
	var result struct {
		Items []struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
		} `json:"items"`
	}
	selector := "app.kubernetes.io/managed-by=harbormaster,harbormaster.io/deployment=" + k.cfg.Deployment + ",harbormaster.io/purpose=workspace"
	if err := k.call(ctx, "GET", "/api/v1/namespaces?labelSelector="+url.QueryEscape(selector), nil, &result); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, ns := range namespaces {
		seen[ns] = true
	}
	for _, item := range result.Items {
		project := item.Metadata.Labels["workbench.project"]
		if workspaceNamespace(project) == "" || k.workspaceNamespace(project) != item.Metadata.Name || item.Metadata.Labels["harbormaster.io/project"] != project || item.Metadata.Labels["workbench.security"] != "enforced" {
			return nil, errors.New("workspace namespace ownership conflict")
		}
		// A failed provisioning attempt may have created only the namespace.
		// Do not let it break lookups for every already-running project.
		var binding map[string]any
		if err := k.call(ctx, "GET", "/apis/rbac.authorization.k8s.io/v1/namespaces/"+item.Metadata.Name+"/rolebindings/workbench-runtime", nil, &binding); err != nil {
			if isNotFound(err) {
				continue
			}
			return nil, err
		}
		if !containsDesired(binding, map[string]any{
			"roleRef":  map[string]any{"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": "harbormaster-workspace-runtime"},
			"subjects": []any{map[string]any{"kind": "ServiceAccount", "name": "runtime-controller", "namespace": k.cfg.CoreNamespace}},
		}) {
			return nil, errors.New("workspace runtime binding conflict")
		}
		if !seen[item.Metadata.Name] {
			namespaces = append(namespaces, item.Metadata.Name)
			seen[item.Metadata.Name] = true
		}
	}
	sort.Strings(namespaces)
	return namespaces, nil
}

func (k *Kubernetes) ensureWorkspaceNamespace(ctx context.Context, project string) error {
	namespace := workspaceNamespace(project)
	if !k.cfg.ProvisionWorkspaceNamespaces || namespace == "" || namespace == k.cfg.CoreNamespace {
		return errors.New("workspace namespace provisioning unavailable")
	}
	labels := map[string]any{"app.kubernetes.io/managed-by": "harbormaster", "harbormaster.io/deployment": k.cfg.Deployment, "harbormaster.io/project": project, "harbormaster.io/purpose": "workspace", "workbench.project": project, "workbench.security": "enforced", "pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "latest"}
	object := func(api, kind, name string) map[string]any {
		return map[string]any{"apiVersion": api, "kind": kind, "metadata": map[string]any{"name": name, "namespace": namespace, "labels": labels}}
	}
	ns := object("v1", "Namespace", namespace)
	delete(ns["metadata"].(map[string]any), "namespace")
	ns["metadata"].(map[string]any)["annotations"] = map[string]any{"argocd.argoproj.io/sync-options": "Prune=false,Delete=false"}
	if err := k.ensureProjectObject(ctx, "/api/v1/namespaces", ns); err != nil {
		return err
	}
	base := "/api/v1/namespaces/" + namespace
	for name, spec := range k.workspacePolicies(project) {
		policy := object("networking.k8s.io/v1", "NetworkPolicy", name)
		policy["spec"] = spec
		if err := k.ensureProjectObject(ctx, "/apis/networking.k8s.io/v1/namespaces/"+namespace+"/networkpolicies", policy); err != nil {
			return err
		}
	}
	quota := object("v1", "ResourceQuota", "workspace-quota")
	quota["spec"] = map[string]any{"hard": map[string]any{"pods": "2", "requests.cpu": "3", "limits.cpu": "3", "requests.memory": "3Gi", "limits.memory": "3Gi", "requests.storage": "40Gi", "persistentvolumeclaims": "4", "services.loadbalancers": "0", "services.nodeports": "0"}}
	if err := k.ensureProjectObject(ctx, base+"/resourcequotas", quota); err != nil {
		return err
	}
	var registry map[string]any
	if err := k.call(ctx, "GET", "/api/v1/namespaces/"+k.cfg.CoreNamespace+"/secrets/harbormaster-registry", nil, &registry); err != nil {
		return err
	}
	data, ok := registry["data"].(map[string]any)
	if !ok || registry["type"] != "kubernetes.io/dockerconfigjson" || data[".dockerconfigjson"] == nil {
		return errors.New("registry credentials unavailable")
	}
	secret := object("v1", "Secret", "harbormaster-registry")
	secret["type"] = registry["type"]
	secret["data"] = data
	if err := k.ensureProjectObject(ctx, base+"/secrets", secret); err != nil {
		return err
	}
	account := object("v1", "ServiceAccount", k.cfg.ServiceAccount)
	account["automountServiceAccountToken"] = false
	account["imagePullSecrets"] = []any{map[string]any{"name": "harbormaster-registry"}}
	if err := k.ensureProjectObject(ctx, base+"/serviceaccounts", account); err != nil {
		return err
	}
	binding := object("rbac.authorization.k8s.io/v1", "RoleBinding", "workbench-runtime")
	binding["subjects"] = []any{map[string]any{"kind": "ServiceAccount", "name": "runtime-controller", "namespace": k.cfg.CoreNamespace}}
	binding["roleRef"] = map[string]any{"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": "harbormaster-workspace-runtime"}
	return k.ensureProjectObject(ctx, "/apis/rbac.authorization.k8s.io/v1/namespaces/"+namespace+"/rolebindings", binding)
}
