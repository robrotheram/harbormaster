# k3s reference environment

Fresh single-node reference installation at `https://harbormaster.exceptionerror.io`,
on context `default`, node `ds9.exceptionerror.io`. The local Compose environment
is independent and remains intact. This is not a high-availability production install.

## Build and apply

Run from the repository root. The complete desired application state is this
Kustomize overlay; there are no generated platform JSON, image-lock JSON or
endpoint discovery files to update.

```sh
kubectl --context default get nodes
kubectl kustomize deploy/kubernetes/overlays/reference > /tmp/harbormaster.yaml
kubectl --context default apply --dry-run=server -k deploy/kubernetes/overlays/reference
kubectl --context default diff -k deploy/kubernetes/overlays/reference
kubectl --context default apply -k deploy/kubernetes/overlays/reference
```

Review differences first. `kubectl diff` exits 1 when changes are present.
This layout uses one core namespace, `harbormaster`, and separate workspace
namespaces. It is **not an in-place rename** of the earlier two-core-namespace
installation. Existing installations must follow [the migration procedure](namespace-migration.md)
before applying this overlay. Applying directly would initialize a new empty database,
leave existing Secrets behind and conflict with the reserved gateway Service IPs.
The reference cluster completed this migration on 2026-11-15. The retired
`workbench-system` and `workbench-egress` namespaces were subsequently deleted.
The old database PV remains retained, alongside a private logical database backup
and resource inventory. Rollback now requires restoring resources and database
state; the previous application can no longer simply be scaled back up.

For an empty reference environment, install the external prerequisites, then:

```sh
kubectl --context default apply -k deploy/kubernetes/overlays/reference/bootstrap
kubectl --context default -n harbormaster wait cluster/harbormaster-db \
  --for=condition=Ready --timeout=5m
```

Provision the Secrets below, then apply the complete overlay. The bootstrap and
complete overlay deliberately share the same resources; do not create separate
Argo Applications that compete for ownership of them.

## Shared workspace

The reference controlplane sets `SHARED_WORKSPACE_ID=ws_shared`. All projects use
that one T3/Codex workspace, its persistent files, agent home, history and network
settings. Project work boards and scheduling metadata remain separately addressed;
projects are organizational labels, not filesystem isolation boundaries in this mode.
Use project-specific folders to organize repositories in the shared filesystem.

Creating a project or submitting work does not allocate another workspace or
namespace. The worker is provisioned idempotently at controlplane startup. The
workspace namespace quota permits one running Pod. Its CPU and memory budget
includes the Kata VM overhead. Existing stopped workspace
volumes are retained for recovery; they are not adopted or deleted automatically.
Branch workers, additional agent types and deletion of the shared worker are
unavailable in shared mode. Remove the explicit setting only through a reviewed
configuration change if separate workspaces are needed later.

## Dependencies and ownership

- `harbormaster`: core platform, gateways, CNPG PostgreSQL 17, dashboard TLS and auth.
- `workbench-reference`: restricted project policy, retained workspace state.
- Existing Longhorn 1.11.2, CloudNativePG 1.29.1, cert-manager with
  `letsencrypt-prod`, Traefik and oauth2-proxy are cluster-owned prerequisites.
- Agent Sandbox v1.0.9 is separately installed in `agent-sandbox-system`.
- Kata chart 4.3.1 is separately owned by Helm release `harbormaster-kata` in
  `kata-system`; see [infrastructure](../../infrastructure/README.md).

The explicit Longhorn `harbormaster-retain` class has one replica and Retain
reclaim policy. This avoids the cluster's two default classes and accidental PVC
reclamation. It does not protect against node/disk loss or replace backups.

`gateway-services-patch.yaml` declares the existing gateway ClusterIPs. The runtime
patch uses those exact IPs because workspace policy intentionally blocks general
recursive DNS. These values are specific to this cluster's Service CIDR; update
both patches together for a different cluster. Do not independently recreate a
Service with another IP. The trusted platform still uses normal cluster DNS.

## Secret contract

Existing reference Secrets are retained and are **not** managed by this overlay.
No credentials were migrated from local development. For a fresh install, obtain
values through the approved secret manager or prepare files in a private directory
with mode 0700 and files mode 0600. Never place plaintext values in Git, Kustomize
literals, shell command arguments or Argo Application parameters.

| Namespace | Secret | Required file keys |
| --- | --- | --- |
| harbormaster | controlplane-secrets | control_token, controller_token, database_url, database_ca.crt |
| harbormaster | runtime-controller-secrets | controller_token, gateway_service_token, grant_secret |
| harbormaster | dashboard-secrets | control_token |
| harbormaster | mcp-secrets | control_token, mcp_token |
| harbormaster | inference-gateway-secrets | gateway_service_token, subscription_vault_token (empty in fixture mode) |
| harbormaster | credential-broker-secrets | gateway_service_token |
| harbormaster | egress-gateway-secrets | gateway_service_token |
| harbormaster | inference-gateway-config | inference-gateway.json |
| harbormaster | credential-broker-config | credential-broker.json |
| harbormaster | egress-gateway-config | empty Secret in fixture mode |
| both namespaces | harbormaster-registry | .dockerconfigjson; type kubernetes.io/dockerconfigjson |

Use the same control token wherever `control_token` is required, the same controller
token across controlplane/runtime, and the same gateway token across runtime and
gateways. Each distinct token and the grant secret should be independently random
(at least 32 bytes). The existing `harbormaster-identity` Secret records the current
reference identity; preserve it and matching per-service values on adoption.
Changing a grant secret invalidates workspace grants and requires coordinated migration.

Use native `kubectl create secret ... --from-file` or the secret manager to satisfy
these contracts. For example, on a **new** installation with prepared files:

```sh
kubectl --context default -n harbormaster create secret generic controlplane-secrets \
  --from-file=control_token=/secure/harbormaster/control_token \
  --from-file=controller_token=/secure/harbormaster/controller_token \
  --from-file=database_url=/secure/harbormaster/database_url \
  --from-file=database_ca.crt=/secure/harbormaster/database_ca.crt

kubectl --context default -n harbormaster create secret generic inference-gateway-config \
  --from-file=inference-gateway.json=config/inference.json
kubectl --context default -n harbormaster create secret generic credential-broker-config \
  --from-file=credential-broker.json=config/broker.json
kubectl --context default -n harbormaster create secret generic egress-gateway-config

for ns in harbormaster workbench-reference; do
  kubectl --context default -n "$ns" create secret generic harbormaster-registry \
    --type=kubernetes.io/dockerconfigjson \
    --from-file=.dockerconfigjson=/secure/harbormaster/registry.json
done
```

Create the other service Secrets using their exact keys in the table. `create`
fails if a Secret already exists; do not replace the live identity on reruns.
The registry file must hold narrowly scoped GHCR `read:packages` credentials.
The overlay sets imagePullSecrets on both platform Pods and the workspace SA.

CNPG generates `harbormaster-db-app` and `harbormaster-db-ca`. Prepare the database
URL from that application's username/password with proper URL encoding, using
`harbormaster-db-rw.harbormaster.svc:5432/workbench`,
`sslmode=verify-full&sslrootcert=/run/secrets/database_ca.crt`; copy `ca.crt` to the
application's `database_ca.crt` file. Never disable verification. Refresh those
application Secret values and restart controlplane after CNPG credential/CA rotation.

## Images and updates

Platform images are pinned in `../../base/kustomization.yaml`; the workspace image
is pinned in `../../base/runtime-controller.yaml`. The reference release uses the
published images from commit `01ec2ac80727d73eb94b0d546b443216245da1b9`, including
namespace consolidation, native operational tooling and shared-workspace mode.
All nine GitHub container jobs and the release checks passed for this source.
Rebuild any optional Hermes image against the matching workspace base as well.

After a successful GitHub build, obtain the immutable digest from its build output
or authenticated registry inspection. Add an `images` override to this overlay
(or use `kustomize edit set image` in this directory). For example, with a real
reviewed digest:

```yaml
images:
  - name: ghcr.io/robrotheram/harbormaster/dashboard
    digest: sha256:REPLACE_WITH_64_HEX_DIGEST
```

Build, validate, review the diff, commit and apply/sync. Do not select `edge` or
mutable tags. Workspace images are independently configured with the
runtime-controller's `WORKSPACE_IMAGE` environment value. Existing Sandboxes
reject image drift; plan retained-workspace migration before changing that value.

## Authentication and execution

Only dashboard HTTPS is exposed. MCP and controlplane remain ClusterIP services.
The existing `oauth-redirect` handler and namespace-local `harbormaster-auth`
middleware use `https://oidc.exceptionerror.io` / Entra. The dashboard independently
validates session cookies against OAuth2 Proxy, rejects caller identity headers,
and enforces its exact HTTPS origin. This is a shared operator console, not per-user
project authorization. WorkOS and OAuth2 Proxy settings cannot be combined.

The base defaults to `REQUIRE_VERIFIED_ISOLATION=true`. The reference runtime patch
explicitly enables execution with `false` after live acceptance. The application
still reports isolation and networking as **unverified**: manual probes do not
implement continuous attestation. Gateway mode is **fixture**; real provider
calls require separate inference/subscription credentials and external Vault.

Native desktop, browser console and preview links currently use `.localhost`
worker origins and are not remotely routable from this hostname. The management
UI and workspace lifecycle API are the exposed reference surfaces. Remote worker
routing needs separate origins/certificates and capability-authentication design.

## Acceptance and recovery

`make kubernetes-check` validates the declarative deployment offline. Use server
dry-run and diff against the intended cluster to validate CRDs/admission as well.
Deployment contains no custom bootstrap, acceptance, backup or recovery scripts.
Live acceptance uses the platform's operational tooling separately from deployment.

Shared-mode acceptance on 2026-11-15 verified all seven core Deployments ready in
`harbormaster`, a healthy restored database, valid HTTPS and Entra redirection.
Project, workspace and agent-home metadata matched exactly before the new
controlplane started. Two temporary projects saw the same `ws_shared` worker and
agent home, and created distinct instruction files on its shared filesystem.
Those files and the PVC/volume identities survived stop/start unchanged. The T3
console returned HTML through the authenticated API. The single worker ran with
Kata on `ds9.exceptionerror.io` (guest kernel 6.18.56). Gateway connections succeeded;
controlplane, PostgreSQL, cluster API and public Internet connections were blocked.
The Pod quota reported one of one in use, including Kata resource overhead.
Temporary projects and their test files were removed; the shared worker was left
ready. The final live Kustomize diff was empty. The two legacy Sandboxes stayed
suspended with their original PVCs; local Compose was not changed. Subsequent
cleanup removed both retired core namespaces and their old database Pod, while
retaining the old database PV. The active `harbormaster` and `workbench-reference`
namespaces remain in use.

Before namespace consolidation, live acceptance on 2026-11-15 confirmed guest kernel 6.18.56, RWOP second-writer
rejection, retained files after application stop/start, recreated Pods, and Codex/T3
workspace starts. T3 HTML was fetched through the private controlplane/runtime
proxy. Workspace probes reached the labelled gateway and could not reach the API,
another namespace or the Internet. Both named acceptance workspaces were left
stopped with retained volumes. A cold image pull exceeded the bounded API wait;
retrying start after Pod readiness succeeded. Private MCP initialization worked;
anonymous requests and forged hosts were denied.

The operator confirmed Entra sign-in and dashboard loading. Live checks also
confirmed valid TLS, anonymous Entra redirection and forged-header rejection.
External model calls, remote worker links and disaster recovery were not accepted.

Database and volume backup policies, CSI snapshot support and an independent
backup destination still need provisioning through the cluster's operators.
Retain and Longhorn alone are not a backup contract. Pause by scaling platform
Deployments, preserving CNPG, Secrets,
Sandboxes and PVCs; never uninstall Kata/Longhorn as routine rollback. See
[operations](../../../../docs/k3s.md) for recovery and schema rollback limits.

For future Argo adoption use `../../argocd/application.yaml`. It is separate from
this overlay, has no automated sync or cascading-delete finalizer, and must not
be installed as a side effect of an ordinary application deployment.
