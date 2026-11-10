package kubernetes

import (
	"context"
	"errors"
	"net/url"

	"agent-workbench/internal/runtime/protocol"
)

// Verify retained claims before resuming a Sandbox. Never adopt an unlabelled,
// cross-home or differently fenced claim, even if the Sandbox labels match.
func (k *Kubernetes) storageEvidence(ctx context.Context, r kubeResource) error {
	var list struct {
		Items []struct {
			Metadata struct {
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			Spec struct {
				StorageClassName string   `json:"storageClassName"`
				AccessModes      []string `json:"accessModes"`
			} `json:"spec"`
		} `json:"items"`
	}
	selector := url.QueryEscape("workbench.workspace=" + r.Metadata.Labels["workbench.workspace"])
	if err := k.call(ctx, "GET", "/api/v1/namespaces/"+r.Metadata.Namespace+"/persistentvolumeclaims?labelSelector="+selector, nil, &list); err != nil {
		return err
	}
	if len(list.Items) != 1 {
		return errors.New("private storage missing or ambiguous")
	}
	p := list.Items[0]
	if protocol.CheckOwnership(p.Metadata.Labels, protocol.WorkspaceSpec{ProjectID: r.Metadata.Labels["workbench.project"], HomeID: r.Metadata.Labels["workbench.home"]}) != nil || p.Metadata.Labels["workbench.deployment"] != k.cfg.Deployment || p.Metadata.Labels["workbench.generation"] != r.Metadata.Labels["workbench.generation"] || p.Spec.StorageClassName != k.cfg.StorageClass || len(p.Spec.AccessModes) != 1 || p.Spec.AccessModes[0] != "ReadWriteOncePod" {
		return errors.New("private storage ownership or fence drift")
	}
	return nil
}
