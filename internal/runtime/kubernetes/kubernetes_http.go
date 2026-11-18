package kubernetes

import (
	"crypto/subtle"
	"io"
	"net/http"

	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
)

func (k *Kubernetes) Handler() http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if k.call(r.Context(), "GET", "/version", nil, nil) != nil {
			shared.Error(w, 503, "KUBERNETES_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]string{"status": "ok"})
	})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/projects/{id}/namespace", func(w http.ResponseWriter, r *http.Request) {
		namespace, err := k.EnsureProjectNamespace(r.Context(), r.PathValue("id"))
		if err != nil {
			shared.Error(w, 503, "PROJECT_NAMESPACE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]string{"namespace": namespace})
	})
	mux.HandleFunc("GET /v1/capabilities", func(w http.ResponseWriter, r *http.Request) {
		c := protocol.RuntimeCapability{Resources: protocol.ResourceLimits{CPUCores: 1, MemoryBytes: k.cfg.MemoryLimit}, SchemaVersion: 1, Backend: "kubernetes-agent-sandbox", Version: AgentSandboxRelease, ID: protocol.DevelopmentTemplate, AgentKinds: []string{"t3", "codex"}, NetworkModes: []string{"blocked", "allowlist"}, IsolationStatus: "unverified", NetworkStatus: "unverified", PrivateHomes: true}
		if k.hermesEnabled() {
			c.AgentKinds = append(c.AgentKinds, "hermes")
		}
		shared.JSON(w, 200, c)
	})
	mux.HandleFunc("POST /v1/workspaces/{id}/network-policy", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Connections *shared.ConnectionPolicy `json:"connections"`
		}
		if !shared.Decode(w, r, &input, 32768) {
			return
		}
		if input.Connections == nil {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if k.ApplyNetwork(r.Context(), r.PathValue("id"), *input.Connections) != nil {
			shared.Error(w, 503, "NETWORK_POLICY_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]bool{"applied": true})
	})
	mux.HandleFunc("POST /v1/workspaces/{id}/start", func(w http.ResponseWriter, r *http.Request) {
		var spec protocol.WorkspaceSpec
		if !shared.Decode(w, r, &spec, 2048) {
			return
		}
		o, err := k.StartConfigured(r.Context(), r.PathValue("id"), spec)
		if err != nil {
			shared.Error(w, 503, "RUNTIME_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, o)
	})
	mux.HandleFunc("GET /v1/workspaces/{id}", func(w http.ResponseWriter, r *http.Request) {
		o, err := k.Inspect(r.Context(), r.PathValue("id"))
		if err != nil {
			shared.Error(w, 503, "RUNTIME_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, o)
	})
	mux.HandleFunc("POST /v1/workspaces/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		if k.Stop(r.Context(), r.PathValue("id")) != nil {
			shared.Error(w, 409, "WORKSPACE_BUSY")
			return
		}
		shared.JSON(w, 200, map[string]bool{"stopped": true})
	})
	mux.HandleFunc("POST /v1/workspaces/{id}/reconcile-network", func(w http.ResponseWriter, r *http.Request) {
		if k.Reconcile(r.Context(), r.PathValue("id")) != nil {
			shared.Error(w, 503, "RUNTIME_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]bool{"reconciled": true})
	})
	mux.HandleFunc("/v1/workspaces/{id}/console/{path...}", func(w http.ResponseWriter, r *http.Request) { k.Console(w, r, r.PathValue("id"), r.PathValue("path")) })
	mux.HandleFunc("/v1/workspaces/{id}/application-proxy/{path...}", func(w http.ResponseWriter, r *http.Request) {
		k.Application(w, r, r.PathValue("id"), r.PathValue("path"))
	})
	mux.HandleFunc("/v1/workspaces/{id}/desktop/{path...}", func(w http.ResponseWriter, r *http.Request) { k.Desktop(w, r, r.PathValue("id"), r.PathValue("path")) })
	mux.HandleFunc("/v1/workspaces/{id}/agent/{path...}", func(w http.ResponseWriter, r *http.Request) {
		path := "/" + r.PathValue("path")
		if !protocol.AllowedAgentPath(r.Method, path) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		query, err := protocol.AgentQuery(r.Method, path, r.URL.RawQuery)
		if err != nil {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		limit := protocol.AgentBodyLimit(path)
		if query != "" {
			path += "?" + query
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
		if err != nil {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		status, out, err := k.Agent(r.Context(), r.PathValue("id"), r.Method, path, body)
		if err != nil {
			shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_, _ = w.Write(out)
	})
	root.Handle("/v1/", shared.RequireToken(k.cfg.Token, mux))
	root.Handle("POST /v1/grants/verify", shared.RequireToken(k.cfg.GatewayServiceToken, http.HandlerFunc(k.verify)))
	return root
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
				if k.networkEvidence(r.Context(), namespace, item.Metadata.Labels["workbench.project"]) != nil {
					shared.Error(w, 503, "NETWORK_POLICY_DRIFT")
					return
				}
				if request.Audience == "hermes-mcp" && (item.Metadata.Labels["workbench.agent"] != "hermes" || !k.hermesEnabled() || item.Spec.OperatingMode == "Suspended" || len(item.Spec.PodTemplate.Spec.Containers) != 1 || item.Spec.PodTemplate.Spec.Containers[0].Image != k.cfg.Image) {
					shared.Error(w, 403, "GRANT_REJECTED")
					return
				}
				identity := map[string]any{"workspace_id": id, "project_id": item.Metadata.Labels["workbench.project"], "principal_id": "local-operator"}
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
