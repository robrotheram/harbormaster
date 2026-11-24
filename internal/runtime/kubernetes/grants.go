package kubernetes

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"

	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
)

// Stage the Secret while the Sandbox is confirmed suspended. A retry can
// finish the same staged rotation if updating the Sandbox failed.
func (k *Kubernetes) resumeSandbox(ctx context.Context, sandbox kubeResource) error {
	meta := sandbox.Metadata
	labels := meta.Labels
	id := labels["workbench.workspace"]
	secretPath := "/api/v1/namespaces/" + meta.Namespace + "/secrets/" + meta.Name + "-grant"
	var secret struct {
		kubeResource
		Data map[string][]byte `json:"data"`
	}
	if err := k.call(ctx, http.MethodGet, secretPath, nil, &secret); err != nil {
		return err
	}
	secretLabels := secret.Metadata.Labels
	owner := protocol.WorkspaceSpec{ProjectID: labels["workbench.project"], HomeID: labels["workbench.home"]}
	if protocol.CheckOwnership(secretLabels, owner) != nil ||
		secretLabels["workbench.workspace"] != id || secretLabels["workbench.deployment"] != k.cfg.Deployment ||
		secretLabels["workbench.generation"] != labels["workbench.generation"] ||
		meta.ResourceVersion == "" || secret.Metadata.ResourceVersion == "" {
		return errors.New("grant ownership or resource version mismatch")
	}
	generation := grantGeneration(secret.kubeResource)
	if generation == "" || generation == grantGeneration(sandbox) ||
		subtle.ConstantTimeCompare(secret.Data["token"], []byte(k.token(id, generation))) != 1 {
		generation = shared.NewID("gen_")
		patch := map[string]any{
			"metadata": map[string]any{"resourceVersion": secret.Metadata.ResourceVersion, "labels": map[string]string{"workbench.grant-generation": generation}},
			"data":     map[string][]byte{"token": []byte(k.token(id, generation))},
		}
		if err := k.call(ctx, http.MethodPatch, secretPath, patch, nil); err != nil {
			return err
		}
	}
	generationLabels := map[string]string{"workbench.grant-generation": generation}
	patch := map[string]any{
		"metadata": map[string]any{"resourceVersion": meta.ResourceVersion, "labels": generationLabels},
		"spec": map[string]any{
			"operatingMode": "Running",
			"podTemplate":   map[string]any{"metadata": map[string]any{"labels": generationLabels}},
		},
	}
	return k.call(ctx, http.MethodPatch, k.collection(meta.Namespace)+"/"+meta.Name, patch, nil)
}
