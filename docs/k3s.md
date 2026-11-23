# Operate and verify the Kubernetes deployment

Use the [deployment guide](deployment.md#install-the-kubernetes-reference-deployment) to prepare the cluster and overlay. The default verified-isolation requirement blocks worker starts. Check the runtime and network enforcement on your installation before relying on those boundaries.

## Provisioning

Use Kubernetes with stable ValidatingAdmissionPolicy support, network-policy
 enforcement, pinned Agent Sandbox v1.0.9 CRDs/controller, a tested kata-qemu
containerd handler, and snapshot-capable CSI with ReadWriteOncePod. k3s local-path
storage is insufficient. The application is packaged as a native Kustomize base
and environment overlays. Its base includes Sandbox admission; the installed
Kata Helm release owns RuntimeClasses. See the
[deployment guide](../deploy/kubernetes/README.md) and separately managed
[infrastructure](../deploy/kubernetes/infrastructure/README.md).

```sh
kubectl kustomize /path/to/environment
kubectl --context YOUR_CONTEXT apply --server-side --dry-run=server -k /path/to/environment
kubectl --context YOUR_CONTEXT diff -k /path/to/environment
kubectl --context YOUR_CONTEXT apply --server-side -k /path/to/environment
```

Create new environment overlays with native `images` and `patches`. They must
select project workspace bindings or automatic namespace provisioning, a tested
RWOP storage class, authenticated ingress/TLS and immutable image digests. The runtime controller discovers gateway Service IPs from the API. Keep secrets outside Git.
`make kubernetes-check` builds with pinned Kustomize and checks the deployment
contract in CI. There are no Python rendering or bootstrap steps.

The reference uses project-isolated agents in `hm-wrk-*` namespaces, with central
authenticated gateways in `harbormaster`. Applications use separate `hm-prj-*`
namespaces. The old shared workspace is retained stopped for operator recovery. Namespace
creation is idempotent and does not deploy application workloads; use the namespace
shown in project settings with normal Helm/Kustomize/CI. Workspace DNS remains
restricted; the reference uses fixed gateway Service IPs. Platform services use
normal cluster DNS. See the reference runbook for application network policy.

Set `KUBERNETES_WORKER_CPU_REQUEST_MILLIS` on the runtime controller to size the
CPU reservation for new worker Pods. The default is 1000 millicores. The
container limit is one CPU, and Kubernetes adds any Kata runtime overhead to
the request. Update existing worker templates while their Sandboxes are
suspended.

Pre-create SERVICE-secrets for each service in its namespace, with lowercase
keys matching the TOKEN_FILE names in the rendered environment (for example
controller_token). Gateway SERVICE-config Secrets supply inference-gateway.json
and credential-broker.json plus required CA files at /etc/workbench. No provider
credential belongs in workspace manifests. Keep Vault external and backed up.
Supply a PostgreSQL endpoint and configure its TLS connection URL in the
controlplane database_url Secret. The reference runs CloudNativePG. No dashboard or workspace receives it.

The reference exposes dashboard, shared-worker and wildcard preview HTTPS ingress. MCP and the control plane remain cluster-private.
The existing platform token is a service credential, not human authentication or
project membership. An ingress login alone does not implement per-user project
membership. The reference verifies signed-in identity and enforces explicit project
membership before worker or private-output access.
Keep one controlplane replica and Recreate strategy until multi-writer coordination
exists. Use project quotas for bounded CPU/memory/storage; pending capacity must
be visible through the existing controller timeout, never silently upgraded.

## Network and storage operations

Capabilities advertise blocked and allowlist; open remains unsupported. Start
checks the entire namespace NetworkPolicy set against the installed template.
Unknown additive policies and drift fail closed. Network-policy application uses
the existing ConfigureConnections HTTP contract and persists generation-bound
policy in a controller-only Secret. All destinations must be inside the operator's
KUBERNETES_ALLOWLISTS_FILE project ceiling. Missing policy denies egress grants;
blocked denies them too. The gateway receives the exact acknowledged allowlist
through grant verification. Every grant verification rereads policy evidence.
Gateway reachability remains available for inference/broker even when general
connections are blocked. Direct Internet connections are never opened.

KUBERNETES_EGRESS_URL points to the trusted egress gateway. Only that gateway
resolves external targets and enforces destination policy; workspaces receive no
Kubernetes token, arbitrary Pod flags or proxy override. Network status remains
unverified: API configuration is evidence of installed rules, not CNI enforcement.
Admission and RBAC must prevent other principals adding policies between reads.

Resume checks retained PVC project/home/deployment/generation ownership, CSI class
and RWOP fence. Stop quiesces the agent and waits for the current Sandbox generation's
Suspended condition. Do not delete a retained Sandbox/PVC to recover capacity.
Branch seeding is unsupported. For the approved image, model, and MCP setup required to enable Hermes, see [Kubernetes Hermes](k3s-hermes.md).

## Backup and recovery

The deployment contains no custom backup or recovery scripts. Configure database
backups through CloudNativePG and workspace-volume backups through the storage
platform, with an independent backup destination. Retain reclaim policy and snapshots
remaining on the same node do not provide disaster recovery.

A consistent recovery point must cover metadata, workspace files and matching
identity/grant Secrets. Suspend workspaces and verify their observed generation
is suspended before capturing a coordinated recovery point. Restore into an
isolated environment with public ingress disabled, preserving project/home
ownership and RWOP storage. Verify retained file hashes and single-writer
reattachment before resuming execution. Record restore duration and data loss
against explicitly chosen RTO/RPO targets.

## External acceptance gates

Use the platform's operational tooling against a disposable Kata workspace to
verify blocked direct Internet and cross-project/API access, approved gateway
reachability, and the controller's inability to create Pods. The repository's
Kustomize checks validate configuration; they do not replace live acceptance.
Before production record:

1. Host-side Kata VM/process evidence per workspace; reject runc fallback and
   scheduling on ineligible nodes. Verify no guest ServiceAccount token exists.
2. Blocked and exact allowlist requests through the real gateway; denied alternate
   hosts/ports, DNS rebinding/private-IP destinations, proxy bypass and project
   crossover. Remove/mutate policy and verify grants fail immediately.
3. Quota/admission denials, resource exhaustion, cancellation and dependency loss;
   no readiness/verified claims or replayed external writes after interruption.
4. Pod/node loss, observed stop, RWOP second-writer rejection, PVC reattachment,
   cross-home claim rejection and retained file hashes after resume.
5. Revoked/expired human sessions and project membership denial, real TLS, M1–M4
   product demonstrations, then isolated restore of metadata/homes/artifacts.

Required neutral-contract extensions: project endpoint routing for multiple gateway
sets; independently measured isolation/network evidence; explicit admission/pending
failure observations; CSI branch provenance; and continuous verification of human
project identity propagation. No change to the neutral runtime protocol is made here.

## Web preview origins

Hosted Web previews use a separate HTTPS origin per workspace service:
`https://preview-<service-hash>.<dashboard-host>`. The dashboard derives this
address from its configured authenticated public origin, not forwarded headers.
Local development continues to use `http://preview-<service-hash>.localhost:<port>`.

Route `*.<dashboard-host>` to the dashboard Service and provision wildcard DNS
and TLS for it. The reference overlay includes the wildcard ingress and a separate
`harbormaster-previews-tls` Certificate. Its cluster-owned `letsencrypt-prod`
issuer must support DNS-01 for this domain; an HTTP-01-only issuer cannot issue
the wildcard certificate. Point your wildcard DNS record to the ingress controller before using previews. Existing dashboard and worker certificates remain
separate.

Do not attach operator OAuth middleware to preview ingress: each preview uses a
short-lived, single-use ticket and a host-bound session. Keep the dashboard's
operator authentication enabled. Unrecognized wildcard host labels are rejected.
