# Kubernetes deployment

Harbormaster uses native Kustomize resources. No manifest generator, Python,
Helm inflation, exec plugin, or cluster lookup is needed to render the application.

```sh
kubectl kustomize deploy/kubernetes/overlays/reference
kubectl --context default apply --server-side --dry-run=server -k deploy/kubernetes/overlays/reference
kubectl --context default diff -k deploy/kubernetes/overlays/reference
kubectl --context default apply --server-side -k deploy/kubernetes/overlays/reference
```

`kubectl diff` returns 1 when a difference exists; review that output before apply.
The reference overlay preserves the existing deployment's object names and image
digests. Its [runbook](overlays/reference/README.md) records dependencies,
credentials, acceptance evidence and remaining limitations.

## Layout

| Path | Purpose |
| --- | --- |
| `base/` | Platform services plus the sandbox controller, shared workspace policy, runtime bindings and Sandbox admission policy. Workspace execution defaults to blocked; no public ingress. |
| `overlays/reference/` | The existing single-node k3s environment: project policy, CNPG, retained Longhorn storage, TLS, Traefik/Entra integration, fixed gateway IPs and fixture gateways. |
| `overlays/reference/bootstrap/` | Namespace/database foundation for an empty environment, before credentials are provisioned. Also included in the complete reference overlay. |
| `components/hermes/` | Optional dedicated Hermes MCP service; not enabled in the reference overlay. See [Hermes](../../docs/k3s-hermes.md). |
| `infrastructure/` | Vendored Agent Sandbox controller included in the application; cluster prerequisites. Kata remains a Helm release requiring host maintenance; its RuntimeClasses are not application-owned. |
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

The reference has one core namespace, `harbormaster`, including one shared agent
and its sandbox controller. New logical projects get application namespaces named
`hm-prj-<immutable project ID suffix>`. They do not get additional agent workspaces.
Project settings expose the namespace and provisioning status. Resource limits,
network isolation, registry pull credentials and an unprivileged `application`
ServiceAccount are installed idempotently by the runtime controller. Use that
ServiceAccount in application manifests deployed through normal Helm/Kustomize/CI.
Namespace provisioning is separate from building and deploying applications.

The runtime ServiceAccount has namespace/resource provisioning permissions, scoped
registry reads and Sandbox/grant management. It has no direct Pod creation or exec
permission and does not delete project namespaces. Project deletion retains deployed
applications and storage. Agent Sandbox owns worker Pod/PVC/Service reconciliation.
The shared worker uses Kata and private RWOP storage, with no Kubernetes credentials.

Workspace policies select only workspace-labelled Pods within the core namespace.
The one-worker quota is scoped to the `harbormaster-workspace` PriorityClass so core
services are unaffected. The namespace labels and Sandbox admission policy enforce
project/Pod label agreement and the quota class. Unknown network policy drift still
fails closed. See the [reference runbook](overlays/reference/README.md) for details.

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

## Core namespace migration

Core services and gateways share `harbormaster`; agent project namespaces remain
separate. The legacy pinned runtime/workspace images must be updated to builds
from the namespace-consolidation change before deploying this layout. See the
[reference cutover procedure](overlays/reference/namespace-migration.md) for
database, identity, Service IP and retained-workspace migration requirements.
