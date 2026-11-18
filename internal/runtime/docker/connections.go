package docker

import (
	"agent-workbench/internal/runtime/protocol"
	"context"
	"errors"
	"slices"
)

func connectionMode(info containerInfo) string {
	if slices.Contains(info.Config.Env, "WORKSPACE_CONNECTION_MODE=open") {
		return "open"
	}
	return "restricted"
}
func hasConnectionProxy(info containerInfo) bool {
	return slices.Contains(info.Config.Env, "CONNECTION_PROXY_VERSION=2")
}
func (c *Controller) ensureInternetNetwork(ctx context.Context, id string, spec protocol.WorkspaceSpec) (string, error) {
	name := c.name(id) + "-internet"
	var info struct {
		Internal bool
		Labels   map[string]string
	}
	err := c.Docker.call(ctx, "GET", "/networks/"+name, nil, &info)
	if err != nil {
		var de protocol.APIError
		if !errors.As(err, &de) || de.Status != 404 {
			return "", err
		}
		err = c.Docker.call(ctx, "POST", "/networks/create", map[string]any{"Name": name, "Driver": "bridge", "Internal": false, "EnableIPv6": false, "Labels": c.ownedLabels(id, spec)}, nil)
		if err != nil {
			return "", err
		}
	} else if info.Internal || info.Labels["workbench.deployment"] != c.cfg.Deployment || info.Labels["workbench.workspace"] != id || protocol.CheckOwnership(info.Labels, spec) != nil {
		return "", errors.New("internet network ownership mismatch")
	}
	return name, nil
}
