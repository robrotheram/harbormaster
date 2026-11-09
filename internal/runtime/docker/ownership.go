package docker

import "agent-workbench/internal/runtime/protocol"

func (c *Controller) ownedLabels(id string, spec protocol.WorkspaceSpec) map[string]string {
	labels := c.labels(id)
	labels["workbench.project"], labels["workbench.home"] = spec.ProjectID, spec.HomeID
	return labels
}

func RuntimeCapabilities() protocol.RuntimeCapability {
	return protocol.RuntimeCapability{Resources: protocol.ResourceLimits{CPUCores: 1, MemoryBytes: 512 << 20}, SchemaVersion: 1, Backend: "docker", BranchSeed: true, NetworkModes: []string{"blocked", "allowlist", "open"}, ID: protocol.DevelopmentTemplate, Version: "1", AgentKinds: []string{"t3", "codex"}, IsolationStatus: "unverified", NetworkStatus: "unverified", PrivateHomes: true}
}
