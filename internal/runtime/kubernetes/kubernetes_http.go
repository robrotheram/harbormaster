package kubernetes

import (
	"context"
	"crypto/subtle"
	"net/http"

	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
)

func (k *Kubernetes) Handler() http.Handler {
	return protocol.NewControllerHandler(protocol.Backend{
		Token: k.cfg.Token, GatewayServiceToken: k.cfg.GatewayServiceToken,
		Health: k.health, VerifyGrant: k.verify, RegisterManagement: k.registerManagement,
		Start: k.startWorkspace, Stop: k.stopWorkspace, Inspect: k.inspectWorkspace,
		BindNetworkPolicy: protocol.BindNetworkPolicy(k.applyWorkspaceNetwork), Reconcile: k.reconcileWorkspaceNetwork,
		Capabilities: k.capabilities, Proxy: k.proxy, Agent: protocol.BufferedAgent(k.proxyAgent),
		StartBodyLimit: 2048, AgentNoStore: true,
	})
}

func (k *Kubernetes) health(w http.ResponseWriter, r *http.Request) {
	if k.call(r.Context(), "GET", "/version", nil, nil) != nil {
		shared.Error(w, 503, "KUBERNETES_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, map[string]string{"status": "ok"})
}

func (k *Kubernetes) registerManagement(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/projects/{id}/namespace", func(w http.ResponseWriter, r *http.Request) {
		namespace, err := k.EnsureProjectNamespace(r.Context(), r.PathValue("id"))
		if err != nil {
			shared.Error(w, 503, "PROJECT_NAMESPACE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]string{"namespace": namespace})
	})
}

func (k *Kubernetes) capabilities() protocol.RuntimeCapability {
	caps := protocol.DevelopmentCapability("kubernetes-agent-sandbox", AgentSandboxRelease, k.cfg.MemoryLimit)
	if k.hermesEnabled() {
		caps.AgentKinds = append(caps.AgentKinds, "hermes")
	}
	return caps
}

func (k *Kubernetes) startWorkspace(ctx context.Context, id string, spec protocol.WorkspaceSpec) (shared.Observation, *protocol.ControllerError) {
	o, err := k.StartConfigured(ctx, id, spec)
	if err != nil {
		return shared.Observation{}, protocol.Failure(503, "RUNTIME_UNAVAILABLE")
	}
	return o, nil
}

func (k *Kubernetes) inspectWorkspace(ctx context.Context, id string) (shared.Observation, *protocol.ControllerError) {
	o, err := k.Inspect(ctx, id)
	if err != nil {
		return shared.Observation{}, protocol.Failure(503, "RUNTIME_UNAVAILABLE")
	}
	return o, nil
}

func (k *Kubernetes) stopWorkspace(ctx context.Context, id string) *protocol.ControllerError {
	if k.Stop(ctx, id) != nil {
		return protocol.Failure(409, "WORKSPACE_BUSY")
	}
	return nil
}

func (k *Kubernetes) applyWorkspaceNetwork(ctx context.Context, id string, connections *shared.ConnectionPolicy) *protocol.ControllerError {
	if connections == nil {
		return protocol.Failure(400, "INVALID_REQUEST")
	}
	if k.ApplyNetwork(ctx, id, *connections) != nil {
		return protocol.Failure(503, "NETWORK_POLICY_UNAVAILABLE")
	}
	return nil
}

func (k *Kubernetes) reconcileWorkspaceNetwork(ctx context.Context, id string) *protocol.ControllerError {
	if k.Reconcile(ctx, id) != nil {
		return protocol.Failure(503, "RUNTIME_UNAVAILABLE")
	}
	return nil
}

func (k *Kubernetes) proxyAgent(ctx context.Context, id, method, path string, body []byte) (int, []byte, *protocol.ControllerError) {
	status, out, err := k.Agent(ctx, id, method, path, body)
	if err != nil {
		return 0, nil, protocol.Failure(503, "WORKSPACE_UNAVAILABLE")
	}
	return status, out, nil
}

func (k *Kubernetes) verify(w http.ResponseWriter, r *http.Request) {
	if k.cfg.RequireVerified {
		shared.Error(w, 503, "VERIFIED_ISOLATION_UNAVAILABLE")
		return
	}
	var request struct {
		Token    string `json:"token"`
		Audience string `json:"audience"`
	}
	if !shared.Decode(w, r, &request, 2048) {
		return
	}
	if len(request.Token) != 64 || (request.Audience != "inference" && request.Audience != "broker" && request.Audience != "egress" && request.Audience != "hermes-mcp") {
		shared.Error(w, 403, "GRANT_REJECTED")
		return
	}
	namespaces, err := k.runtimeNamespaces(r.Context())
	if err != nil {
		shared.Error(w, 503, "GRANT_VERIFIER_UNAVAILABLE")
		return
	}
	for _, namespace := range namespaces {
		var list struct {
			Items []kubeResource `json:"items"`
		}
		if k.call(r.Context(), "GET", k.collection(namespace), nil, &list) != nil {
			shared.Error(w, 503, "GRANT_VERIFIER_UNAVAILABLE")
			return
		}
		for _, item := range list.Items {
			id := item.Metadata.Labels["workbench.workspace"]
			generation := item.Metadata.Labels["workbench.generation"]
			if generation != "" && k.owned(item, id) && sandboxReady(item) && subtle.ConstantTimeCompare([]byte(request.Token), []byte(k.token(id, generation))) == 1 {
				if k.networkEvidence(r.Context(), namespace, protocol.ProjectOf(item.Metadata.Labels)) != nil {
					shared.Error(w, 503, "NETWORK_POLICY_DRIFT")
					return
				}
				if request.Audience == "hermes-mcp" && (item.Metadata.Labels["workbench.agent"] != "hermes" || !k.hermesEnabled() || item.Spec.OperatingMode == "Suspended" || len(item.Spec.PodTemplate.Spec.Containers) != 1 || item.Spec.PodTemplate.Spec.Containers[0].Image != k.cfg.Image) {
					shared.Error(w, 403, "GRANT_REJECTED")
					return
				}
				identity := map[string]any{"workspace_id": id, "project_id": protocol.ProjectOf(item.Metadata.Labels), "principal_id": "local-operator"}
				if request.Audience == "egress" {
					p, err := k.readPolicy(r.Context(), item)
					if err != nil || p.Mode == "blocked" {
						shared.Error(w, 403, "GRANT_REJECTED")
						return
					}
					identity["connections"] = p
				}
				shared.JSON(w, 200, identity)
				return
			}
		}
	}
	shared.Error(w, 403, "GRANT_REJECTED")
}
