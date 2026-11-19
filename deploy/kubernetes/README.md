# Kubernetes deployment

Harbormaster ships native Kustomize resources for Agent Sandbox and Kata.
Production isolation acceptance is pending. Workspace starts fail closed by
default while verified isolation is required.

| Path | Purpose |
| --- | --- |
| `base/` | Platform services, the Agent Sandbox controller, admission policies, default workspace bindings and core NetworkPolicies. |
| `overlays/example/` | A portable starting point with a placeholder HTTPS host, WorkOS sign-in and isolated workspace namespaces. |
| `components/isolated-workers/` | Scoped provisioning for dynamically created `hm-wrk-*` worker namespaces. |
| `components/hermes/` | Optional dedicated Hermes MCP service. See [Hermes](../../docs/k3s-hermes.md). |
| `infrastructure/` | The bundled Sandbox controller and prerequisites for the separately managed Kata runtime. |

The live k3s reference overlay, host-specific Kata values, Argo CD application
and operating history live in the private [reference environment repository](https://github.com/robrotheram/harbormaster-reference).

## Configure an environment

Start from a copy of the example overlay and pin its remote base and components
to a product release tag. Replace the example HTTPS host, OAuth callback,
IngressClass and ingress controller selectors. Supply the named TLS and WorkOS
Secrets and register the callback with WorkOS. Configure a CSI StorageClass that
supports ReadWriteOncePod and replace `KUBERNETES_STORAGE_CLASS`. Set the
project bindings, PostgreSQL endpoint and registry credentials for the cluster.
The selected example Secret keys are listed in its Deployment patches and base
manifests.

Pin each platform image by digest using the release image inventory and Kustomize
`images`. Set `WORKSPACE_IMAGE` to the matching workspace digest in a runtime
Deployment patch. Existing retained Sandboxes reject image drift. Coordinate
workspace recreation with data retention and rollback before an image change.
For Hermes, use the Hermes workspace digest for both `WORKSPACE_IMAGE` and
`KUBERNETES_HERMES_V023_APPROVED_IMAGE`, set a model, and include the MCP component.

The runtime controller reads named gateway Service ClusterIPs from the API at
startup. Restart it if a gateway Service is recreated with a new address. Existing
Sandboxes retain their pod configuration, so recover or recreate those workers
through the environment's workspace maintenance procedure after a Service IP change.
Worker DNS remains restricted. The base defaults to fixture inference, and real
provider access needs an explicit gateway policy and environment acceptance.

Secrets are provisioned outside Git. Platform ingress policies allow named
service-to-service flows. Restrict any additional dashboard, database or MCP
ingress to the required source pods. Use `workbench.workspace DoesNotExist` on
core-service policy selectors so workspace policy drift checks remain independent.

## Validate

```sh
make kubernetes-check
kubectl kustomize /path/to/environment
kubectl --context YOUR_CONTEXT apply --server-side --dry-run=server -k /path/to/environment
kubectl --context YOUR_CONTEXT diff --server-side -k /path/to/environment
```

The server dry run requires the cluster's operators, server APIs and admission
rules. Review the diff before applying the overlay. Application deployments live
in `hm-prj-*` namespaces and isolated Kata workers use separate `hm-wrk-*`
namespaces with private RWOP storage. Workspace policies and admission enforce
restricted pods, scoped gateway access and project ownership. See the
[Kubernetes operations guide](../../docs/k3s.md) for acceptance and recovery.
