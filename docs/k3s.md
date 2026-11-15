# k3s production operations (M5)

Compose remains development. This package implements production operations but
**does not constitute M5 acceptance**. The initial implementation used simulated infrastructure. A fresh
[k3s reference environment](../deploy/kubernetes/overlays/reference/README.md) now has live
Kata, Longhorn, network-policy and application lifecycle checks. The default verified-isolation gate continues
to reject starts: neither Pod readiness nor a configured RuntimeClass proves a VM.
Do not expose an unverified test install as production.

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
kubectl kustomize deploy/kubernetes/overlays/reference
kubectl --context default apply --server-side --dry-run=server -k deploy/kubernetes/overlays/reference
kubectl --context default diff -k deploy/kubernetes/overlays/reference
kubectl --context default apply --server-side -k deploy/kubernetes/overlays/reference
```

Create new environment overlays with native `images` and `patches`. They must
select the shared workspace binding, a tested RWOP storage class, gateway endpoints,
authenticated ingress/TLS and immutable image digests. Keep secrets outside Git.
`make kubernetes-check` builds with pinned Kustomize and checks the deployment
contract in CI. There are no Python rendering or bootstrap steps.

The reference uses one shared agent and gateway set in `harbormaster`. New projects
receive separate application deployment namespaces, not agent workspaces. Namespace
creation is idempotent and does not deploy application workloads; use the namespace
shown in project settings with normal Helm/Kustomize/CI. Workspace DNS remains
restricted; the reference uses fixed gateway Service IPs. Platform services use
normal cluster DNS. See the reference runbook for application network policy.

Pre-create SERVICE-secrets for each service in its namespace, with lowercase
keys matching the TOKEN_FILE names in the rendered environment (for example
controller_token). Gateway SERVICE-config Secrets supply inference-gateway.json
and credential-broker.json plus required CA files at /etc/workbench. No provider
credential belongs in workspace manifests. Keep Vault external and backed up.
The database is external PostgreSQL; configure its TLS connection URL in the
controlplane database_url Secret. No dashboard or workspace receives it.

Only dashboard HTTPS ingress is exposed. MCP/controlplane remain cluster-private.
The existing platform token is a service credential, not human authentication or
project membership. An ingress login alone does not implement per-user project
membership: remote multi-user management remains gated on that integration.
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
Branch seeding remains unsupported. Hermes remains unadvertised pending native
pinned-image integration acceptance; Hermes adapters are installed from `integrations/hermes`.

## Backup and recovery

The deployment contains no custom backup or recovery scripts. Configure database
backups through CloudNativePG and workspace-volume backups through the storage
platform, with an independent backup destination. These policies are not yet
configured for the reference environment. Retain reclaim policy and snapshots
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
failure observations; CSI branch provenance; and authenticated human project
identity propagation. No change to the neutral runtime protocol is made here.
