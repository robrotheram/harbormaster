package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
)

// NetworkPolicy is additive: reject unknown policies rather than assuming a
// default deny cancels an independently installed allow rule.
func (k *Kubernetes) networkEvidence(ctx context.Context, namespace, project string) error {
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec map[string]any `json:"spec"`
		} `json:"items"`
	}
	if err := k.call(ctx, "GET", "/apis/networking.k8s.io/v1/namespaces/"+namespace+"/networkpolicies", nil, &list); err != nil {
		return err
	}
	expected := projectPolicies(project)
	if len(list.Items) != len(expected) {
		return errors.New("network policy set drift")
	}
	for _, item := range list.Items {
		want, ok := expected[item.Metadata.Name]
		if !ok || !reflect.DeepEqual(item.Spec, want) {
			return errors.New("network policy drift")
		}
		delete(expected, item.Metadata.Name)
	}
	if len(expected) != 0 {
		return errors.New("network policy missing")
	}
	return nil
}
func projectPolicies(project string) map[string]map[string]any {
	// Decode canonical JSON to match Kubernetes' unstructured numeric types.
	peer := func(role string) any {
		return map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]string{"kubernetes.io/metadata.name": "harbormaster"}}, "podSelector": map[string]any{"matchLabels": map[string]string{"workbench.project": project, "workbench.role": role}}}
	}
	selector := map[string]any{"matchLabels": map[string]string{"workbench.project": project}}
	rules := []any{}
	for _, r := range []struct {
		role  string
		ports []any
	}{{"dns", []any{map[string]any{"protocol": "UDP", "port": 53}, map[string]any{"protocol": "TCP", "port": 53}}}, {"gateway", []any{map[string]any{"protocol": "TCP", "port": 8080}}}, {"registry", []any{map[string]any{"protocol": "TCP", "port": 8081}}}} {
		rules = append(rules, map[string]any{"to": []any{peer(r.role)}, "ports": r.ports})
	}
	raw := map[string]any{
		"workspace-default-deny":       map[string]any{"podSelector": map[string]any{}, "policyTypes": []string{"Ingress", "Egress"}},
		"workspace-controller-ingress": map[string]any{"podSelector": selector, "policyTypes": []string{"Ingress"}, "ingress": []any{map[string]any{"from": []any{map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]string{"kubernetes.io/metadata.name": "harbormaster"}}, "podSelector": map[string]any{"matchLabels": map[string]string{"app": "runtime-controller"}}}}, "ports": []any{map[string]any{"protocol": "TCP", "port": 8080}}}}},
		"workspace-project-egress":     map[string]any{"podSelector": selector, "policyTypes": []string{"Egress"}, "egress": rules},
	}
	b, _ := json.Marshal(raw)
	var out map[string]map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}
func (k *Kubernetes) policyPath(r kubeResource) string {
	return "/api/v1/namespaces/" + r.Metadata.Namespace + "/secrets/" + r.Metadata.Name + "-network"
}
func (k *Kubernetes) readPolicy(ctx context.Context, r kubeResource) (shared.ConnectionPolicy, error) {
	var s struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Data map[string][]byte `json:"data"`
	}
	var p shared.ConnectionPolicy
	if err := k.call(ctx, "GET", k.policyPath(r), nil, &s); err != nil {
		return p, err
	}
	if !reflect.DeepEqual(s.Metadata.Labels, r.Metadata.Labels) {
		return p, errors.New("network policy ownership drift")
	}
	if json.Unmarshal(s.Data["policy"], &p) != nil || !k.allowedPolicy(r.Metadata.Labels["workbench.project"], p) {
		return p, errors.New("invalid network policy")
	}
	return p, nil
}
func (k *Kubernetes) allowedPolicy(project string, p shared.ConnectionPolicy) bool {
	if !p.Valid() || p.Mode == "open" {
		return false
	}
	for _, d := range p.Destinations {
		found := false
		for _, a := range k.cfg.ProjectAllowlists[project] {
			if a == d {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func (k *Kubernetes) ApplyNetwork(ctx context.Context, id string, p shared.ConnectionPolicy) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	r, err := k.find(ctx, id)
	if err != nil {
		return err
	}
	project := r.Metadata.Labels["workbench.project"]
	if !k.allowedPolicy(project, p) {
		return errors.New("project destination denied")
	}
	if err = k.networkEvidence(ctx, r.Metadata.Namespace, project); err != nil {
		return err
	}
	// A failed update leaves the last acknowledged policy intact. Never report success.
	b, _ := json.Marshal(p)
	body := map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": r.Metadata.Name + "-network", "namespace": r.Metadata.Namespace, "labels": r.Metadata.Labels}, "type": "Opaque", "data": map[string][]byte{"policy": b}}
	var existing kubeResource
	err = k.call(ctx, "GET", k.policyPath(r), nil, &existing)
	if err == nil {
		if !reflect.DeepEqual(existing.Metadata.Labels, r.Metadata.Labels) {
			return errors.New("network ownership mismatch")
		}
		return k.call(ctx, "PATCH", k.policyPath(r), body, nil)
	}
	// Only NotFound allows creation.
	if !isNotFound(err) {
		return err
	}
	return k.call(ctx, "POST", "/api/v1/namespaces/"+r.Metadata.Namespace+"/secrets", body, nil)
}

func isNotFound(err error) bool {
	var e protocol.APIError
	return errors.As(err, &e) && e.Status == 404
}
