package kubernetes

import (
	"context"
	"fmt"
	"net"
	"net/netip"
)

func (k *Kubernetes) serviceIP(ctx context.Context, name string) (string, error) {
	var service struct {
		Spec struct {
			ClusterIP string `json:"clusterIP"`
		} `json:"spec"`
	}
	path := "/api/v1/namespaces/" + k.cfg.CoreNamespace + "/services/" + name
	if err := k.call(ctx, "GET", path, nil, &service); err != nil {
		return "", fmt.Errorf("discover gateway service %s: %w", name, err)
	}
	ip, err := netip.ParseAddr(service.Spec.ClusterIP)
	if err != nil || !ip.IsGlobalUnicast() || ip.Zone() != "" {
		return "", fmt.Errorf("gateway service %s needs a routable ClusterIP", name)
	}
	return ip.String(), nil
}

func (k *Kubernetes) discoverGateways(ctx context.Context) error {
	for name, target := range map[string]*string{
		"inference-gateway": &k.cfg.InferenceURL,
		"credential-broker": &k.cfg.BrokerURL,
		"egress-gateway":    &k.cfg.EgressURL,
	} {
		ip, err := k.serviceIP(ctx, name)
		if err != nil {
			return err
		}
		*target = "http://" + net.JoinHostPort(ip, "8080")
	}
	if k.hermesEnabled() {
		ip, err := k.serviceIP(ctx, "hermes-mcp")
		if err != nil {
			return err
		}
		k.cfg.HermesMCPIP = ip
	}
	return nil
}
