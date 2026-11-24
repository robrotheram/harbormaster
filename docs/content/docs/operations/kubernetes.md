+++
title = "Kubernetes deployment"
aliases = ["/docs/reference/deploy/kubernetes/readme/"]
+++

Harbormaster ships native Kustomize resources for Agent Sandbox and Kata.
Start with the [deployment guide](../deployment.md#install-the-kubernetes-reference-deployment). The default verified-isolation requirement blocks worker starts until the runtime can verify isolation.

| Path | Purpose |
| --- | --- |
| [`base/`](../../../../deploy/kubernetes/base) | Platform services, the Agent Sandbox controller, admission policies, default workspace bindings and core NetworkPolicies. |
| [`overlays/example/`](../../../../deploy/kubernetes/overlays/example) | A portable starting point with a placeholder HTTPS host, WorkOS sign-in and isolated workspace namespaces. |
| [`components/isolated-workers/`](../../../../deploy/kubernetes/components/isolated-workers) | Scoped provisioning for dynamically created `hm-wrk-*` worker namespaces. |
| [`components/hermes/`](../../../../deploy/kubernetes/components/hermes) | Optional dedicated Hermes MCP service. See [Hermes](../k3s-hermes.md). |
| [`infrastructure/`](../../../../deploy/kubernetes/infrastructure) | The bundled Sandbox controller and prerequisites for the separately managed Kata runtime. |

The live k3s reference overlay, host-specific Kata values, Argo CD application
and operating history live in the private [reference environment repository](https://github.com/robrotheram/harbormaster-reference).

## Configure an environment

Start from a copy of the example overlay and pin its remote base and components
to a product release tag. Replace the example HTTPS host, OAuth callback,
IngressClass and ingress controller selectors. Supply the named TLS Secret. Configure hosted dashboard access as described below. Set `KUBERNETES_RUNTIME_CLASS` to the installed Kata RuntimeClass. Set `KUBERNETES_STORAGE_CLASS` to a CSI StorageClass that supports ReadWriteOncePod. Set the
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
Worker DNS remains restricted. The base sets `GATEWAY_MODE=upstream` on the inference gateway and credential broker. For a synthetic installation, patch those Deployments to `fixture` and supply the fixture policies from [config](gateway-configuration.md). For upstream access, supply the corresponding policy and scoped credentials for each service.

Secrets are provisioned outside Git. Platform ingress policies allow named
service-to-service flows. Restrict any additional dashboard, database or MCP
ingress to the required source pods. Use `workbench.workspace DoesNotExist` on
core-service policy selectors so workspace policy drift checks remain independent.

## Provision Secrets

Create the platform Secrets in the `harbormaster` namespace through your secret manager. Read the rendered Deployments for any additional keys that your overlay references.

| Secret | Required values and matching credentials |
| --- | --- |
| `controlplane-secrets` | Supply `database_url`, `control_token`, and `controller_token`. For hosted dashboard and project access, supply the scoped values below. |
| `runtime-controller-secrets` | Supply `controller_token` matching the control plane, plus `grant_secret` and `gateway_service_token`. |
| `inference-gateway-secrets`, `credential-broker-secrets`, and `egress-gateway-secrets` | Supply the controller's `gateway_service_token`. Add the provider or Vault credentials required by each gateway's configuration. |
| `inference-gateway-config` and `credential-broker-config` | Supply `inference-gateway.json` and `credential-broker.json` respectively. Include CA files if the policy needs them. |
| `egress-gateway-config` | Create the Secret if the Deployment retains the base volume reference. The current egress service gets its destination policy from the runtime grant. |
| `dashboard-secrets` | Supply the scoped credentials below and the WorkOS keys if you use the example's AuthKit patch. |
| `mcp-secrets` | Supply `mcp_token` for the default project and `project_signing_key` matching the control plane. |

## Configure hosted dashboard access

1. Provision `dashboard_read_token` and `dashboard_human_signing_key` as separate random values of at least 32 characters. Use identical values in `controlplane-secrets` and `dashboard-secrets`. Keep the control token and credential encryption key in the control-plane Secret.
2. In the dashboard container's `env` patch, add the following entries. Keep the example's WorkOS entries when you use WorkOS.

   ```yaml
   - name: DASHBOARD_HUMAN_SIGNING_KEY
     valueFrom:
       secretKeyRef:
         name: dashboard-secrets
         key: dashboard_human_signing_key
   ```

   The base already mounts the dashboard read token. The base control-plane Deployment reads both scoped values from its own Secret.
3. Provision the separate `project_signing_key` with the same value on control plane and MCP. Supply the same key to the optional Hermes MCP service when included. Provision `credential_encryption_key` for the control plane if you store integration credentials.
4. Configure [WorkOS](../authkit.md) or replace the WorkOS settings with the OAuth2 Proxy settings in that guide. Set `PROJECT_CREATOR_EMAILS` on the control plane if approved users need to create projects. Grant users access through project membership.
5. Check the rendered dashboard Deployment for the scoped credentials. Confirm its Secret excludes `control_token`. Test access with an invited user and with an approved user who has no membership in the project.

## Validate

```sh
make kubernetes-check
kubectl kustomize /path/to/environment
kubectl --context YOUR_CONTEXT apply --server-side --dry-run=server -k /path/to/environment
kubectl --context YOUR_CONTEXT diff --server-side -k /path/to/environment
```

The server dry run requires the cluster's operators, server APIs and admission
rules. Review the diff before applying the overlay. The example provisions project application namespaces named `hm-prj-*` and uses separate `hm-wrk-*` namespaces for isolated workers with private ReadWriteOncePod storage. Workspace policies and admission enforce
restricted pods, scoped gateway access and project ownership. See the
[Kubernetes operations guide](../k3s.md) for acceptance and recovery.
