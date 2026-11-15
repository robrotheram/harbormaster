# Agent Sandbox runtime backend

This is a deployment foundation, not a certification of VM or network isolation. The adapter has fake-API tests and an in-progress [k3s reference installation](reference/README.md) with live Kata, storage and network-policy probes. Full application acceptance is tracked separately.

The implementation targets Kubernetes SIGs Agent Sandbox **v1.0.9**, `agents.x-k8s.io/v1beta1`. It creates `Sandbox` resources with an administrator-fixed digest image, `RuntimeClass`, service account and `volumeClaimTemplates`. Agent Sandbox owns Pod/PVC/Service reconciliation. Stop requests `operatingMode: Suspended` and waits for the observed suspension condition; the Sandbox/PVC persist. Deletion/retention is intentionally a separate operator action.

Primary API source: <https://github.com/kubernetes-sigs/agent-sandbox/blob/v1.0.9/api/v1beta1/sandbox_types.go>. Install its pinned controller/CRDs using that release's instructions, not floating `main` manifests.

## Prerequisites

- Kubernetes admission support for `ValidatingAdmissionPolicy`, a tested network-policy-enforcing CNI, and project namespaces provisioned by an administrator.
- Kata/containerd configured on eligible virtualization-capable nodes. Review `runtimeclass.yaml` and verify the `kata-qemu` handler. Label/taint only nodes with actual tested VM support. EKS instance eligibility, nested virtualization and storage/CNI behavior need a separate host validation.
- A private built workspace image addressed by SHA-256 digest, and a CSI StorageClass supporting **ReadWriteOncePod**. This is the volume generation fence; no two writable workspace Pods may attach the same state.
- Project-specific DNS, inference/broker and Nexus egress endpoints. The default policies permit only appropriately labelled internal endpoints; until those exist, access fails closed. The resolver must answer approved internal service names and deny arbitrary recursive DNS. No external DNS fallback, IP egress or proxy override is opened by these templates.
- The trusted runtime controller can connect to the workspace Service on port 8080; workspaces cannot reach the Kubernetes API or other project Pods.

## Provision a project

Create the project through the workbench API first. Render and review its namespace policy:

```sh
python3 deploy/kubernetes/render_project.py --project-id proj_RETURNED_ID \
  --namespace workbench-example --output /tmp/workbench-project.yaml
kubectl apply -f deploy/kubernetes/admission.yaml
kubectl apply -f deploy/kubernetes/runtimeclass.yaml
kubectl apply -f /tmp/workbench-project.yaml
```

The runtime controller ServiceAccount is `runtime-controller` in `workbench-system`. The generated RoleBinding gives it only Sandbox lifecycle and per-workspace grant Secret operations in that project namespace. It has no `pods/exec`, Pod creation, host or namespace-creation permission. Agent Sandbox's separate upstream controller has its own required infrastructure RBAC; neither credential belongs in a workspace.

Mount a private JSON file mapping real project IDs to preprovisioned namespaces, for example `{"proj_returned_id":"workbench-example"}`. A namespace can be assigned to only one project. New projects cannot provision compute until this binding and their policies exist.

## Runtime-controller configuration

Run the existing runtime-controller image with these settings instead of mounting Docker:

| Variable | Value |
| --- | --- |
| `RUNTIME_BACKEND` | `kubernetes` |
| `KUBERNETES_API_SERVER` | `https://kubernetes.default.svc` or reviewed HTTPS API origin |
| `KUBERNETES_TOKEN_FILE` | Controller ServiceAccount token file; defaults to normal in-cluster path |
| `KUBERNETES_CA_FILE` | Trusted API CA file; defaults to normal in-cluster path |
| `KUBERNETES_PROJECTS_FILE` | Administrator JSON project-to-namespace bindings |
| `WORKSPACE_IMAGE` | Fixed built image with `@sha256:<64 hex>` |
| `KUBERNETES_RUNTIME_CLASS` | `kata`; keep admission policy in agreement |
| `KUBERNETES_STORAGE_CLASS` | Explicit tested RWOP-capable CSI class |
| `KUBERNETES_WORKSPACE_SERVICE_ACCOUNT` | `workspace` |
| `KUBERNETES_INFERENCE_URL` | Approved internal inference gateway origin |
| `KUBERNETES_BROKER_URL` | Approved internal credential broker origin |
| `CODEX_MODEL` | Exact subscription-approved model ID |
| `REQUIRE_VERIFIED_ISOLATION` | Defaults `true`; starts remain blocked until a verifier exists. Set `false` only for the explicitly unverified orchestration test environment. |

Supply the existing controller/gateway/grant secret files through `CONTROLLER_TOKEN_FILE`, `GATEWAY_SERVICE_TOKEN_FILE` and `GRANT_SECRET_FILE`. No secret values belong in manifests. The public control plane still uses the authenticated runtime-controller HTTP API and receives no Kubernetes credential.

## Current capability limits

- Native T3/Codex workspaces, HTTP/WebSocket forwarding, project/home ownership, persistent state, durable tasks and generation-bound grants are implemented. CPU/memory samples remain unknown until a real metrics adapter is configured.
- Kubernetes branch seeding returns `KUBERNETES_BRANCH_SEED_UNAVAILABLE` before creation. Docker branch copying remains available. A Kubernetes data mover or snapshot/clone integration needs explicit CSI and provenance policy rather than broad cross-PVC mounts.
- Hermes is not falsely advertised as a supported workspace engine; use the separate project assistant recipe in `../hermes` until native adapter compatibility is validated.
- No warm-pool assignment or automatic dirty-guest reuse is implemented. Adopt the upstream claim/warm-pool API only after testing data-free capacity binding. Existing used Sandbox/PVC objects are never reassigned to a different project/home.
- API acceptance of RuntimeClass and Pod readiness are not VM attestation. Isolation and network evidence always report `unverified`; the secure gate never silently falls back to runc.

The adapter now advertises blocked/allowlist and implements persisted policy application,
project destination ceilings and installed NetworkPolicy drift checks. Network status
remains unverified. See [production operations](../../docs/k3s.md) for packaging,
configuration, recovery and the outstanding external acceptance gates.
