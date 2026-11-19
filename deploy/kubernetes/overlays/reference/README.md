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
The core namespace, `harbormaster`, contains platform services, PostgreSQL,
Agent Sandbox controller and stopped legacy agents. Each project uses a separate
`hm-wrk-*` namespace for workers and `hm-prj-*` namespace for deployed applications.
The cluster's existing storage, ingress, authentication and Kata host runtime remain independently managed prerequisites.

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

## Project-isolated workers

`SHARED_WORKSPACE_ID` is empty. Signed-in users access projects through explicit
membership; project ownership is also checked before worker, history, file,
preview and subscription-load operations.

`PROVISION_WORKSPACE_NAMESPACES=true` creates `hm-wrk-<project ID suffix>` when a
project first starts a worker. These namespaces are distinct from application
namespaces. Each receives restricted admission, a token-free workspace account,
private retained RWOP claims, the registry pull credential, a scoped runtime role
binding and the canonical deny-by-default worker network policies. The controller
discovers owned namespaces after restart; no generated project-binding file is
needed. Namespace ownership conflicts fail closed rather than adopting resources.

Each namespace permits two worker Pods (including Kata overhead within 3 CPU and
3 GiB), four claims and 40 GiB of storage. The controlplane's global 2 GiB worker
memory budget normally permits two 1 GiB workers across all projects. Stopping an
idle worker frees compute while preserving its files. Namespace creation does not
start compute. Kata branch seeding remains unsupported.

`KUBERNETES_SHARED_GATEWAY_PROJECT=default` points worker network rules at the
central gateway service Pods. Each request still requires a workspace-scoped grant;
this does not permit worker-to-worker, application, database, Kubernetes API or
direct Internet traffic. Project destination allowlists remain explicitly managed
in runtime-bindings; new projects start with blocked outbound connections.

The former `ws_shared` worker and its mixed-project data are retained, stopped,
under the operator-only `default` legacy project. Do not grant project membership
to that legacy project or copy its whole volume into a private project. Its old
acceptance-project work records are retained for operator recovery; new work uses
fresh project-owned workers and subscription selections. Saved personal accounts
are retained and users choose which account to load into their new worker.

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
traffic and CoreDNS. Other project namespaces, worker namespaces, core services and
public destinations are blocked by default. Add explicit application-specific
outbound NetworkPolicies in the application deployment when external access is
required; a broad Internet rule can also allow node-hosted cluster services.

Deploy built application manifests with normal Helm/Kustomize or project CI into
this namespace; namespace provisioning does not automatically build or deploy an
application. Existing in-workspace preview APIs remain local previews. Workers
receive no Kubernetes token. Registry credential rotation must also update
provisioned project Secrets through the secret manager.

Project deletion retains worker and application namespaces, deployed apps
and storage. Retiring deployed applications is a separate explicit operator action.

## Dependencies and ownership

- `harbormaster`: platform, gateways, CNPG PostgreSQL 17, TLS/auth, sandbox controller
  and retained legacy agents.
- `hm-wrk-*`: project-isolated agents and their persistent storage.
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

The reference platform and workspace images are pinned to Harbormaster
`v0.2.1` (source `c61052d`). The platform digests are in
`kustomization.yaml` and the workspace digest is in
`runtime-controller-patch.yaml`. The Codex policy and worker default
continue to select `gpt-6.1-sol`. The base retains its independently
pinned default release.

The reference cutover uses a fresh `workbench_v021` database with schema
version 1. Existing workspaces were suspended. The old database and PVCs
are separate from the clean instance.

Reference enables Hermes v0.23.0 in workspace mode using the reviewed
Hermes-enabled workspace image built on `v0.2.1`, a controller with the
private MCP hostname mapping, and `gpt-6.1-sol`. The private `hermes-mcp`
Service uses a pinned ClusterIP so Kata workers can reach it through the
existing gateway policy with DNS blocked. The smoke test started a Kata
workspace and completed a generation-authenticated, project-scoped MCP
call. Model inference and Matrix were outside that check. See
[the Kubernetes Hermes guide](../../../../docs/k3s-hermes.md).

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

When upgrading to the gateway-only workspace egress policy, coordinate the
NetworkPolicy update with a digest for a runtime-controller image built with the
same policy change. Quiesce workspace operations during the rollout because the
controller requires exact policy specs.
Apply the rendered static policies and remove the DNS and registry entries from
`workspace-project-egress` in each existing controller-provisioned workspace
namespace. Preserve the gateway rule's project selector and TCP port 8080.
The upgraded controller creates the reduced policy in new namespaces. Apply the
inverse policy changes if rolling back the controller image.

## Authentication and execution

Dashboard and separate per-worker HTTPS origins are exposed. MCP and controlplane remain ClusterIP services.
The existing `oauth-redirect` handler and namespace-local `harbormaster-auth`
middleware use `https://oidc.exceptionerror.io` / Entra. The dashboard independently
validates session cookies against OAuth2 Proxy, rejects caller identity headers,
and enforces its exact HTTPS origin. Verified signed-in identities are subject to explicit project membership. WorkOS and OAuth2 Proxy settings cannot be combined.

The base defaults to `REQUIRE_VERIFIED_ISOLATION=true`. The reference runtime patch
explicitly enables execution with `false` after live acceptance. The application
still reports isolation and networking as **unverified**: manual probes do not
implement continuous attestation. The inference gateway uses **subscription** mode:
users save a current Codex ChatGPT login under Workers → Subscription, then load
it into a worker owned by their project. See [subscription setup](../../../../docs/subscription-agents.md).
Credential-broker and egress fixture modes remain unchanged.

Browser console and desktop pairing use `ws-<id>.harbormaster.exceptionerror.io`
and `desktop-ws-<id>.harbormaster.exceptionerror.io`. Wildcard DNS and the preview
wildcard certificate cover new workers. Their ingress deliberately has
no operator OAuth middleware: the dashboard requires a one-use browser ticket or
native desktop bearer. Cookies are secure and host-only. Worker origins cannot
access the operator API. Application preview hosting remains separately configured.

For a worker-image upgrade, stop each idle worker through the application API, update
its suspended Sandbox image and `CODEX_MODEL` environment entry to match the
controller defaults, then start it through the application API. Preserve the
Sandbox UID, grant generation, existing claim, home ID and project state. Changing
controller defaults alone does not rewrite an existing Sandbox template. The
workspace supervisor repairs empty initial model selections while retaining
explicit model choices.

## Acceptance and recovery

The 2026-11-18 isolation migration deployed controlplane and runtime-controller
from `b75f8fe` after the complete CI check and both image publishing jobs passed.
Two projects received distinct `hm-wrk-*` namespaces, retained RWOP claims and
agent homes. Both ran with Kata and without Kubernetes tokens. Live connections
in both directions to the other worker, controlplane, Kubernetes API and public
Internet were blocked; the inference gateway remained reachable.

The Sample project worker is Ready with its owner's existing ChatGPT subscription
selected. Personal account listing, model catalog and authenticated T3 HTML passed,
including after a runtime-controller restart. Nonmembers could not access its
worker or history; Sample project membership could not access another project's
worker or `ws_shared`. Another identity saw no saved personal accounts. The
acceptance worker was stopped and its temporary project archived, with disks
retained. No provider inference was requested during these checks.

The old shared worker remains suspended, with its Sandbox UID, PVC UIDs and PV
bindings unchanged. A private database backup and pre-cutover file hashes were
retained. All platform Deployments and PostgreSQL were healthy; HTTPS remained
valid and the final server-side overlay diff was empty. These are point-in-time
acceptance checks, not continuous isolation attestation.

The subscription repair on 2026-11-15 verified a managed Codex task through the
shared worker, its configured outbound-proxy environment, the inference gateway,
and the saved ChatGPT subscription. Task `task_c5f85569e74b7b99aec8f46d0541ddb8`
succeeded with `SUBSCRIPTION_OK` using `gpt-6.1-sol`. The provider rejected the
previous `gpt-5.4` setting; the replacement was selected from the account's live
model catalog. Gateway DNS handling now connects only to approved resolved IPs
without rejecting a permitted IPv4 address because DNS also returns IPv6.
Worker/T3 proxy exclusions include the configured platform endpoint hosts,
including fixed Service IPs; unrelated destinations still use the outbound proxy.

The worker was stopped through the application API before updating its retained
Sandbox image and model setting. Console/T3 defaults were migrated from the old
model with private backups retained. Sandbox UID, PVC UID, home ID and all 21
workspace file hashes were preserved through both restarts. The dashboard also
includes the worker-filter wrapping fix required by the enlarged-text browser
check. Go race tests, vet, Kubernetes validation, CI checks including all 129
browser tests, and the deployed image publishing jobs passed.

The 2026-11-15 Connections rollout was verified live against the source above.
All seven platform deployments and the retained shared worker became Ready.
Schema migration 14 completed. Live acceptance created two synthetic accounts for
each provider, renamed them, confirmed private metadata and cross-user deletion
denial, and removed them without creating worker bindings. No real subscription
was imported or charged; provider inference still requires a user's current login.
The native T3 console responded, hosted browser/desktop TLS and authentication
checks passed, and all workspace file checksums and the existing PVC/PV matched
the pre-rollout snapshots. The worker template now includes both `CODEX_MODEL`
(`gpt-5.4`) and `CLAUDE_MODEL` (`claude-sonnet-5-5`). Existing T3 provider preferences
remain preserved.

Private database, deployment, Sandbox, PVC, and file-checksum backups were taken
before rollout. Schema 14 replaces the legacy single-account tables: rolling back
to an older controlplane image also requires its matching database backup.

The 2026-11-15 personal-subscription rollout was verified live: all seven platform
deployments and the shared worker became Ready; the subscription API required a
verified user identity, rejected API-key imports, and kept credential resolution
separate from control authority. Both worker origins presented valid TLS, browser
access required a ticket, and desktop session inspection remained unauthenticated
without native credentials. The native T3 console responded through controlplane.
The runtime, console and initial T3 selection use `gpt-5.4`. All 19 workspace file
checksums matched before and after the upgrade, with the same PVC and PV. No user
subscription was imported or charged during acceptance; a real Codex turn still
requires the user to save and load a current ChatGPT login.

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
