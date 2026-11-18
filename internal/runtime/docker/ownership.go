package docker

import "agent-workbench/internal/runtime/protocol"

func (c *Controller) ownedLabels(id string, spec protocol.WorkspaceSpec) map[string]string {
	labels := c.labels(id)
	labels["workbench.project"], labels["workbench.home"] = spec.ProjectID, spec.HomeID
	return labels
}

func RuntimeCapabilities() protocol.RuntimeCapability {
	caps := protocol.DevelopmentCapability("docker", "1", 512<<20)
	caps.BranchSeed = true
	caps.NetworkModes = append(caps.NetworkModes, "open")
	return caps
}
