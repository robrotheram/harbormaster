# Separately managed cluster dependencies

The application owns its sandbox controller. Shared cluster operators and the host
runtime retain their existing lifecycle.
The reference already has Longhorn, CloudNativePG, cert-manager, Traefik and Entra
OAuth2 Proxy. Preserve their existing owners when adopting Harbormaster into Argo CD.

## Agent Sandbox

Agent Sandbox v1.0.9 is included in the application base. The controller, service
and service account use `harbormaster`; no separate installation namespace is needed.
The vendored `agent-sandbox/upstream.yaml` is the unmodified official release asset:
`https://github.com/kubernetes-sigs/agent-sandbox/releases/download/v1.0.9/sandbox.yaml`,
SHA-256 `3eeb27fa821659593ba92c5454571f08c84abb07096bb9d372c1c6c750f6d7d2`.
Kustomize removes the upstream namespace, rewrites RBAC subjects, pins the controller
image digest and adds resource/security/probe configuration. The CRD is protected
against pruning and uses server-side apply. Review upstream changes before upgrades;
do not run another controller installation with a separate leader-election namespace.

## Kata

Kata is installed using the official `kata-deploy-4.3.1.tgz` release chart and the
committed `kata/values.yaml`. Release `harbormaster-kata` in `kata-system` owns the
`kata` and `kata-qemu` RuntimeClasses. The values target the one approved k3s node,
use QEMU and preserve the default container runtime.

Host/containerd changes can restart k3s. Use a maintenance window for install,
upgrade or uninstall; never add the install Job as an application sync hook.
After reviewing/verifying the official chart archive:

```sh
helm upgrade --install harbormaster-kata ./kata-deploy-4.3.1.tgz \
  --kube-context default --namespace kata-system --create-namespace \
  -f deploy/kubernetes/infrastructure/kata/values.yaml --timeout 15m
```

The chart owns both RuntimeClasses. Do not replace them with application manifests
or inferred node selectors; keep host/runtime changes in the Helm maintenance lifecycle.
