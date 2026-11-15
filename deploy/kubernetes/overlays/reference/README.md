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
kubectl --context default apply --server-side --dry-run=server -k deploy/kubernetes/overlays/reference
kubectl --context default diff --server-side -k deploy/kubernetes/overlays/reference
kubectl --context default apply --server-side -k deploy/kubernetes/overlays/reference
```

Review differences first. `kubectl diff` exits 1 when changes are present.
The application has one namespace, `harbormaster`: platform services, PostgreSQL,
Agent Sandbox controller and the shared agent workspace. New project namespaces
contain deployed applications only. The cluster's existing storage, ingress,
authentication and Kata host runtime remain independently managed prerequisites.

Existing installations must follow [the migration procedure](namespace-migration.md)
when moving a retained workspace. A namespace cannot be renamed, and applying
Kustomize does not move PVCs or copy Secrets. The former `workbench-system` and
`workbench-egress` namespaces have already been retired; their old database PV and
private recovery records were retained.

For an empty reference environment, install the external prerequisites, then:

```sh
kubectl --context default apply --server-side -k deploy/kubernetes/overlays/reference/bootstrap
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

The worker is provisioned idempotently at controlplane startup. Creating projects
never creates additional agents. Its PriorityClass-scoped quota permits one worker
Pod, including Kata VM overhead, without applying that quota to core services.
Workspace network policies select workspace-labelled Pods, so sharing the namespace
does not grant the agent access to the database, platform API or Kubernetes API.
Branch workers, additional agent types and deletion of the shared worker remain
unavailable in shared mode.

## Project application namespaces

`PROVISION_PROJECT_NAMESPACES=true` on controlplane and runtime-controller makes
project creation prepare `hm-prj-<project ID suffix>` in this same cluster. Names
come from immutable generated IDs, never user-supplied names. Project settings and
the API expose `deployment_namespace` and `namespace_ready`. A provisioning failure
retains the project creation key; retry creation or use **Retry namespace setup**.

Each application namespace receives restricted Pod admission, a LimitRange,
CPU/memory/storage/Pod quotas, isolated networking, the scoped GHCR pull Secret and
an `application` ServiceAccount with token mounting disabled. Application manifests
should use `serviceAccountName: application` and run as non-root. Ingress allows
same-project traffic and the cluster's Traefik Pods. Egress allows same-project
traffic and CoreDNS. Other project namespaces, the shared worker, core services and
public destinations are blocked by default. Add explicit application-specific
outbound NetworkPolicies in the application deployment when external access is
required; a broad Internet rule can also allow node-hosted cluster services.

Deploy built application manifests with normal Helm/Kustomize or project CI into
this namespace; namespace provisioning does not automatically build or deploy an
application. Existing in-workspace preview APIs remain local previews. The shared
agent receives no Kubernetes token. Registry credential rotation must also update
provisioned project Secrets through the secret manager.

Project deletion retains the shared agent, application namespace, deployed apps
and storage. Retiring deployed applications is a separate explicit operator action.

## Dependencies and ownership

- `harbormaster`: platform, gateways, CNPG PostgreSQL 17, TLS/auth, sandbox controller
  and the one persistent shared agent.
- `hm-prj-*`: application deployment destinations created for new projects.
- Existing Longhorn 1.11.2, CloudNativePG 1.29.1, cert-manager with
  `letsencrypt-prod`, Traefik and oauth2-proxy are cluster-owned prerequisites.
- Agent Sandbox v1.0.9 is vendored, digest-pinned and included in the application
  Kustomization. Its CRD is protected against Argo pruning/deletion.
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
| harbormaster | harbormaster-registry | .dockerconfigjson; type kubernetes.io/dockerconfigjson |

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

kubectl --context default -n harbormaster create secret generic harbormaster-registry \
  --type=kubernetes.io/dockerconfigjson \
  --from-file=.dockerconfigjson=/secure/harbormaster/registry.json
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
published images from commit `e1f59f2b5ba121fc7f5f1a4943628fb08f312070`, including
the shared agent and Sandbox controller in the core namespace, and automatic
application namespace provisioning for new projects.
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

Dashboard and separate shared-worker HTTPS origins are exposed. MCP and controlplane remain ClusterIP services.
The existing `oauth-redirect` handler and namespace-local `harbormaster-auth`
middleware use `https://oidc.exceptionerror.io` / Entra. The dashboard independently
validates session cookies against OAuth2 Proxy, rejects caller identity headers,
and enforces its exact HTTPS origin. This is a shared operator console, not per-user
project authorization. WorkOS and OAuth2 Proxy settings cannot be combined.

The base defaults to `REQUIRE_VERIFIED_ISOLATION=true`. The reference runtime patch
explicitly enables execution with `false` after live acceptance. The application
still reports isolation and networking as **unverified**: manual probes do not
implement continuous attestation. The inference gateway uses **subscription** mode:
users save a current Codex ChatGPT login under Workers → Subscription, then load
it into the shared worker. See [subscription setup](../../../../docs/subscription-agents.md).
Credential-broker and egress fixture modes remain unchanged.

Browser console and desktop pairing use `ws-shared.harbormaster.exceptionerror.io`
and `desktop-ws-shared.harbormaster.exceptionerror.io`. DNS must resolve both to
the ingress; the certificate includes both names. Their ingress deliberately has
no operator OAuth middleware: the dashboard requires a one-use browser ticket or
native desktop bearer. Cookies are secure and host-only. Worker origins cannot
access the operator API. Application preview hosting remains separately configured.

After upgrading, restart the idle shared worker to receive `CODEX_MODEL`. The
workspace supervisor repairs empty initial model selections while retaining
explicit model choices. Existing image pins must be updated to builds containing
these changes before applying this overlay; configuration alone cannot add the UI.

## Acceptance and recovery

`make kubernetes-check` validates the declarative deployment offline. Use server
dry-run and diff against the intended cluster to validate CRDs/admission as well.
Deployment contains no custom bootstrap, acceptance, backup or recovery scripts.
Live acceptance uses the platform's operational tooling separately from deployment.

Final consolidation acceptance on 2026-11-15 verified all eight Deployments,
PostgreSQL and the single shared Kata workspace ready in `harbormaster`. The
workspace retained its original backing PV, agent-home identity and all 20 checked
file hashes; its new namespace requires a new PVC identity. The Sandbox controller
adopted that claim. The workspace quota counted only the shared worker, and gateway
connections succeeded while database, controlplane, Kubernetes API and public
Internet connections remained blocked. Runtime-controller could read the registry
pull Secret but could not read database credentials or create Pods directly.

Two temporary projects automatically received distinct application namespaces,
quotas, network policies and application ServiceAccounts. Creation retries and
namespace repair preserved their identities; both projects used the same agent
and home. Project deletion retained the application namespaces as designed, then
the operator removed those test namespaces. An application Pod successfully pulled
the private image and served HTTP during provisioning acceptance; DNS worked while
core services, Kubernetes API and Internet access were blocked, and no Kubernetes
token was mounted. HTTPS and Entra redirection remained valid, the authenticated
T3 console returned HTML, and the server-side Kustomize diff was empty.

`workbench-reference` and `agent-sandbox-system` were removed after migration.
Together with the earlier retirement of `workbench-system` and `workbench-egress`,
this leaves one Harbormaster component namespace. The two legacy workspace PVs and
old database PV remain retained for recovery; private database and workspace
backups were taken before moving the shared volume. Local Compose was unchanged.

Before the final workspace consolidation, shared-mode acceptance on 2026-11-15 verified all seven core Deployments ready in
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
namespaces were then in use; the final consolidation described above completed
the move into `harbormaster`.

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
