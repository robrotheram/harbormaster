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
A first adoption may roll Pods because Kustomize orders patched environment
entries differently; values, images, identities and retained volumes stay the same.
This packaging migration does not reinstall Kata or transfer ownership to Argo CD.
The migration was checked against all 36 prior resources and the live cluster.
Server-side dry-run using the future Argo field manager passed without forcing
conflicts; admission-policy API defaults are explicit to preserve atomic fields.

For an empty reference environment, install the external prerequisites, then:

```sh
kubectl --context default apply -k deploy/kubernetes/overlays/reference/bootstrap
kubectl --context default -n workbench-system wait cluster/harbormaster-db \
  --for=condition=Ready --timeout=5m
```

Provision the Secrets below, then apply the complete overlay. The bootstrap and
complete overlay deliberately share the same resources; do not create separate
Argo Applications that compete for ownership of them.

## Dependencies and ownership

- `workbench-system`: platform, CNPG PostgreSQL 17, dashboard TLS and auth.
- `workbench-egress`: dedicated gateways for project `default`.
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
| workbench-system | controlplane-secrets | control_token, controller_token, database_url, database_ca.crt |
| workbench-system | runtime-controller-secrets | controller_token, gateway_service_token, grant_secret |
| workbench-system | dashboard-secrets | control_token |
| workbench-system | mcp-secrets | control_token, mcp_token |
| workbench-egress | inference-gateway-secrets | gateway_service_token, subscription_vault_token (empty in fixture mode) |
| workbench-egress | credential-broker-secrets | gateway_service_token |
| workbench-egress | egress-gateway-secrets | gateway_service_token |
| workbench-egress | inference-gateway-config | inference-gateway.json |
| workbench-egress | credential-broker-config | credential-broker.json |
| workbench-egress | egress-gateway-config | empty Secret in fixture mode |
| all three namespaces | harbormaster-registry | .dockerconfigjson; type kubernetes.io/dockerconfigjson |

Use the same control token wherever `control_token` is required, the same controller
token across controlplane/runtime, and the same gateway token across runtime and
gateways. Each distinct token and the grant secret should be independently random
(at least 32 bytes). The existing `harbormaster-identity` Secret records the current
reference identity; preserve it and matching per-service values on adoption.
Changing a grant secret invalidates workspace grants and requires coordinated migration.

Use native `kubectl create secret ... --from-file` or the secret manager to satisfy
these contracts. For example, on a **new** installation with prepared files:

```sh
kubectl --context default -n workbench-system create secret generic controlplane-secrets \
  --from-file=control_token=/secure/harbormaster/control_token \
  --from-file=controller_token=/secure/harbormaster/controller_token \
  --from-file=database_url=/secure/harbormaster/database_url \
  --from-file=database_ca.crt=/secure/harbormaster/database_ca.crt

kubectl --context default -n workbench-egress create secret generic inference-gateway-config \
  --from-file=inference-gateway.json=config/inference.json
kubectl --context default -n workbench-egress create secret generic credential-broker-config \
  --from-file=credential-broker.json=config/broker.json
kubectl --context default -n workbench-egress create secret generic egress-gateway-config

for ns in workbench-system workbench-egress workbench-reference; do
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
`harbormaster-db-rw.workbench-system.svc:5432/workbench`,
`sslmode=verify-full&sslrootcert=/run/secrets/database_ca.crt`; copy `ca.crt` to the
application's `database_ca.crt` file. Never disable verification. Refresh those
application Secret values and restart controlplane after CNPG credential/CA rotation.

## Images and updates

Platform images are pinned in `../../base/kustomization.yaml`. The deployed
platform images originate from commit `9e0238383601cd1f81ca411a91077a4cf66d5f24`,
with the authenticated dashboard from `8edda8f89beefbb19b32a72b1f253889be1b3199`.
Both source builds passed their required checks; all nine images were published.

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
`../../acceptance.sh` records infrastructure and probes an operator-provisioned
Kata workspace; it requires kubectl, jq, and curl in the disposable probe image.
It neither installs the platform nor claims complete VM attestation.

Live acceptance on 2026-11-15 confirmed guest kernel 6.18.56, RWOP second-writer
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

CSI VolumeSnapshot APIs/controller and an independent backup destination still
need provisioning before the generic backup/recover scripts can be used. Those
scripts now require jq rather than Python. Retain and Longhorn alone are not a
backup contract. Pause by scaling platform Deployments, preserving CNPG, Secrets,
Sandboxes and PVCs; never uninstall Kata/Longhorn as routine rollback. See
[operations](../../../../docs/k3s.md) for recovery and schema rollback limits.

For future Argo adoption use `../../argocd/application.yaml`. It is separate from
this overlay, has no automated sync or cascading-delete finalizer, and must not
be installed as a side effect of an ordinary application deployment.
