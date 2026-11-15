# Kubernetes deployment

Harbormaster uses native Kustomize resources. No manifest generator, Python,
Helm inflation, exec plugin, or cluster lookup is needed to render the application.

```sh
kubectl kustomize deploy/kubernetes/overlays/reference
kubectl --context default apply --dry-run=server -k deploy/kubernetes/overlays/reference
kubectl --context default diff -k deploy/kubernetes/overlays/reference
kubectl --context default apply -k deploy/kubernetes/overlays/reference
```

`kubectl diff` returns 1 when a difference exists; review that output before apply.
The reference overlay preserves the existing deployment's object names and image
digests. Its [runbook](overlays/reference/README.md) records dependencies,
credentials, acceptance evidence and remaining limitations.

## Layout

| Path | Purpose |
| --- | --- |
| `base/` | Seven platform services, namespace boundaries, runtime bindings and Sandbox admission policy. Workspace execution defaults to blocked; no public ingress. |
| `overlays/reference/` | The existing single-node k3s environment: project policy, CNPG, retained Longhorn storage, TLS, Traefik/Entra integration, fixed gateway IPs and fixture gateways. |
| `overlays/reference/bootstrap/` | Namespace/database/project foundation for an empty environment, before credentials are provisioned. Also included in the complete reference overlay. |
| `components/hermes/` | Optional dedicated Hermes MCP service; not enabled in the reference overlay. See [Hermes](../../docs/k3s-hermes.md). |
| `infrastructure/` | Separately managed cluster dependencies. Kata remains a Helm release requiring host maintenance; its RuntimeClasses are not application-owned. |
| `argocd/` | Future manual-adoption Application pointing directly to the reference overlay. It is not installed by applying the application. |

The base is reusable configuration, not a standalone production installation.
Create another overlay to select project bindings, storage, gateway endpoints,
image pull credentials and authenticated ingress. Use standard `patches` and
`images`; do not edit generated build output. The reference's explicit
`REQUIRE_VERIFIED_ISOLATION=false` is for the accepted test environment only.

## Images and credentials

Platform image digests live in `base/kustomization.yaml`; environment-specific
updates belong in the consuming overlay's `images` entries. The workspace image
is the `WORKSPACE_IMAGE` environment value on the runtime controller and must be
changed explicitly in a reviewed Deployment patch. Existing retained Sandboxes
reject image drift; updating a dashboard does not upgrade workspace images.

Secrets are pre-provisioned outside Git. Kustomize references their names and
keys; it does not generate, rotate, export or commit credentials. For future
Argo CD adoption, supply those same Secret contracts through an approved secret
manager or provision them beforehand. The [secret contract](overlays/reference/README.md#secret-contract)
includes database CA verification and registry credentials.

## Projects and isolation

Create project metadata through the application API, then add a reviewed project
namespace/policy manifest to your overlay. Use
`overlays/reference/foundation/project.yaml` as the concrete example. Change the
namespace and every `workbench.project` label/selector together, and update the
runtime's `projects.json` and `allowlists.json` ConfigMap patch. Keep the RoleBinding
subject in `workbench-system`; a global Kustomize `namespace` transformer is
inappropriate for this multi-namespace application.

One project currently owns each gateway set. Supporting additional projects
requires dedicated gateway/DNS/registry endpoints and runtime endpoint routing;
simply relabelling shared gateways does not establish tenant isolation.

The runtime ServiceAccount can manage Sandboxes and generation-grant Secrets in
its bound project, and read PVC/network-policy evidence. It cannot create or exec
Pods, create namespaces, change network policy or delete retained volumes.
Agent Sandbox v1.0.9 owns Pod/PVC/Service reconciliation. Workspaces use Kata,
private RWOP storage and no Kubernetes token. Stop suspends a Sandbox; deletion
and retention remain separate operator operations.

Isolation and network status remain `unverified`: readiness and a RuntimeClass
are not continuous VM/CNI attestation. Branch seeding and automatic dirty-workspace
reuse remain unsupported. Hermes workspace support requires separate image and
native integration acceptance. See [operations](../../docs/k3s.md).

## Validation and future Argo CD adoption

`make kubernetes-check` requires Go, installs Kustomize v5.8.1 into ignored `.local/tools`,
builds the manifests offline, and checks pinned images, secret exclusion,
credential boundaries, default execution gating, gateway IP consistency and
manual Argo adoption. GitHub's `make check` and GitLab's Go job both run it.
Server dry-run additionally requires the real cluster's CRDs and admission rules.

The [Argo Application](argocd/application.yaml) uses the private GitHub repository,
`main`, and `deploy/kubernetes/overlays/reference`. Register repository credentials
and verify the Argo project permits the namespaces and cluster-scoped resources.
Review the initial adoption diff before syncing. Automated sync and cascading
delete are absent. Namespaces, CNPG and the StorageClass carry
`Prune=false,Delete=false`; generated workspace state and Secrets are outside this
Application. Do not add host-install jobs to its sync lifecycle.

[Kubernetes Kustomize documentation](https://kubernetes.io/docs/tasks/manage-kubernetes-objects/kustomization/)
and [Argo sync options](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-options/)
describe the native mechanisms used here.
