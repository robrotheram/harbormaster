package kubernetes

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"agent-workbench/internal/shared"
)

func KubernetesConfigFromEnv() (KubernetesConfig, error) {
	c := KubernetesConfig{APIURL: shared.Env("KUBERNETES_API_SERVER", "https://kubernetes.default.svc"), CAFile: shared.Env("KUBERNETES_CA_FILE", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"), Deployment: shared.Env("DEPLOYMENT_ID", "awb"), Image: shared.Env("WORKSPACE_IMAGE", ""), RuntimeClass: shared.Env("KUBERNETES_RUNTIME_CLASS", "kata"), StorageClass: shared.Env("KUBERNETES_STORAGE_CLASS", ""), ServiceAccount: shared.Env("KUBERNETES_WORKSPACE_SERVICE_ACCOUNT", "workspace"), InferenceURL: shared.Env("KUBERNETES_INFERENCE_URL", ""), BrokerURL: shared.Env("KUBERNETES_BROKER_URL", ""), CodexModel: shared.Env("CODEX_MODEL", ""), ClaudeModel: shared.Env("CLAUDE_MODEL", ""), MemoryLimit: shared.IntEnv("WORKSPACE_MEMORY_BYTES", 1<<30), RequireVerified: shared.Env("REQUIRE_VERIFIED_ISOLATION", "true") == "true"}
	request, err := strconv.Atoi(shared.Env("KUBERNETES_WORKER_CPU_REQUEST_MILLIS", "1000"))
	if err != nil || request < 1 || request > 1000 {
		return c, errors.New("KUBERNETES_WORKER_CPU_REQUEST_MILLIS must be between 1 and 1000")
	}
	c.CPURequestMillis = request
	c.ProvisionProjectNamespaces = shared.Env("PROVISION_PROJECT_NAMESPACES", "false") == "true"
	c.ProvisionWorkspaceNamespaces = shared.Env("PROVISION_WORKSPACE_NAMESPACES", "false") == "true"
	c.SharedGatewayProject = os.Getenv("KUBERNETES_SHARED_GATEWAY_PROJECT")
	c.CoreNamespace = shared.Env("KUBERNETES_CORE_NAMESPACE", "harbormaster")
	c.DiscoverGateways = shared.Env("KUBERNETES_DISCOVER_GATEWAYS", "false") == "true"
	c.HermesApprovedImage = os.Getenv("KUBERNETES_HERMES_V023_APPROVED_IMAGE")
	c.HermesModel = os.Getenv("HERMES_MODEL")
	c.HermesMCPIP = os.Getenv("KUBERNETES_HERMES_MCP_IP")
	c.EgressURL = shared.Env("KUBERNETES_EGRESS_URL", "")
	if path := os.Getenv("KUBERNETES_ALLOWLISTS_FILE"); path != "" {
		b, e := os.ReadFile(path)
		if e != nil || json.Unmarshal(b, &c.ProjectAllowlists) != nil {
			return c, errors.New("invalid project allowlists")
		}
	}
	if c.Token, err = shared.Secret("CONTROLLER_TOKEN"); err != nil {
		return c, err
	}
	if c.GatewayServiceToken, err = shared.Secret("GATEWAY_SERVICE_TOKEN"); err != nil {
		return c, err
	}
	if c.GrantSecret, err = shared.Secret("GRANT_SECRET"); err != nil {
		return c, err
	}
	b, err := os.ReadFile(shared.Env("KUBERNETES_TOKEN_FILE", "/var/run/secrets/kubernetes.io/serviceaccount/token"))
	if err != nil {
		return c, errors.New("Kubernetes token unavailable")
	}
	c.APIToken = strings.TrimSpace(string(b))
	f, err := os.Open(shared.Env("KUBERNETES_PROJECTS_FILE", "/etc/workbench/project-namespaces.json"))
	if err != nil {
		return c, errors.New("project namespace bindings unavailable")
	}
	defer f.Close()
	if json.NewDecoder(io.LimitReader(f, 64<<10)).Decode(&c.ProjectNamespaces) != nil {
		return c, errors.New("invalid project namespace bindings")
	}
	return c, nil
}
