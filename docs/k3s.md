# k3s production operations (M5)

Compose remains development. This package implements production operations but
**does not constitute M5 acceptance**. No k3s, Kata host, CSI or kubectl was
available during implementation. The default verified-isolation gate continues
to reject starts: neither Pod readiness nor a configured RuntimeClass proves a VM.
Do not expose an unverified test install as production.

## Provisioning

Use Kubernetes with stable ValidatingAdmissionPolicy support, network-policy
 enforcement, pinned Agent Sandbox v1.0.9 CRDs/controller, a tested kata-qemu
containerd handler, and snapshot-capable CSI with ReadWriteOncePod. k3s local-path
storage is insufficient. Apply deploy/kubernetes/admission.yaml and the reviewed
runtimeclass.yaml only after validating the node handler. Render project-template
with render_project.py and apply it before binding that project to the runtime.
Each project gets a unique namespace, restricted Pod security and quota.

render_platform.py CONFIG.json --output platform.json renders standard Kubernetes
objects. Apply with kubectl after reviewing the output. Configuration requires:

- project_id, project_namespace, storage_class, host, tls_secret; allowlist is a
  list of exact host:port destinations (default empty).
- images: digest references for controlplane, runtime-controller, dashboard, mcp,
  inference-gateway, credential-broker, egress-gateway and workspace. Build these
  from the repository's existing Dockerfile targets.
- auth_annotations: annotations configuring the installed ingress authentication
  middleware. Install that middleware separately and test rejection of anonymous,
  expired and revoked sessions. TLS secret must hold the host's valid certificate.
- environment: per-service reviewed settings, including live inference/broker
  policy, external Vault TLS origins/CA paths, models and external DATABASE_URL
  secret. Environment overrides are operator configuration, never guest input.

The renderer currently packages one project per gateway set; do not relabel a
shared gateway for multiple projects. For multiple projects, provision dedicated
DNS/gateway/registry endpoints and extend runtime project endpoint routing first.
Project DNS must resolve the .svc endpoints and refuse arbitrary recursive DNS.
Provision it explicitly; the project policy intentionally does not allow CoreDNS
or Internet DNS by default. The controller and platform use normal cluster DNS.

Pre-create SERVICE-secrets for each service in its namespace, with lowercase
keys matching the TOKEN_FILE names in the rendered environment (for example
controller_token). Gateway SERVICE-config Secrets supply inference-gateway.json
and credential-broker.json plus required CA files at /etc/workbench. No provider
credential belongs in workspace manifests. Keep Vault external and backed up.
The database is external PostgreSQL; configure its TLS connection URL in the
controlplane database_url Secret. No dashboard or workspace receives it.

Only dashboard HTTPS ingress is rendered. MCP/controlplane remain cluster-private.
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
pinned-image integration acceptance; no Hermes adapter scripts are changed here.

## Backup and recovery

Suspend all workspaces through the runtime API first. Run backup.sh from a trusted
host with DATABASE_URL, PROJECT_NAMESPACE, SNAPSHOT_CLASS and BACKUP_DIR. It stops
the sole metadata writer, verifies observed suspension, dumps metadata, captures
identity Secrets and creates/waits for CSI snapshots. It intentionally leaves the
controlplane stopped. Encrypt backup files, export snapshots to independent storage
using the CSI provider, and back up Vault, bindings, policy and TLS configuration.
A snapshot remaining only in the source cluster is not disaster recovery.

Use recover.sh on an isolated recovery cluster with public ingress disabled. Set
BACKUP_DIR, RECOVERY_DATABASE_URL (an empty database) and RESTORED_PVC_MANIFEST
(JSON List). The operator maps exported snapshots to restored VolumeSnapshots;
each PVC must preserve its original name/namespace/ownership and RWOP class.
The script verifies backup checksums, restores metadata and applies snapshot PVCs.
Restore reviewed identity Secrets and suspended Sandbox definitions from the backup
without stale resourceVersion/uid/status fields. Verify file hashes, retained
artifacts and project ownership before enabling controlplane and resuming one
workspace. Record actual recovery duration and last recovered transaction against
an operator-defined RTO/RPO; neither target is assumed by these scripts.

## External acceptance gates

Run acceptance.sh with PROJECT_NAMESPACE, WORKSPACE_POD, OTHER_PROJECT_ENDPOINT,
APPROVED_ENDPOINT and EVIDENCE_DIR against a disposable actual Kata workspace. It
records configuration and probes blocked direct Internet, cross-project/API access,
approved service reachability and the controller's inability to create Pods.
It requires curl inside that test image and operator-only exec permission. These
probes alone are not full acceptance. Before production additionally record:

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
