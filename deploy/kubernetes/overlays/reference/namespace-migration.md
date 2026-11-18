# Historical namespace consolidation

These procedures record the shared-worker consolidation completed on 2026-11-15.
Ref moved to project-isolated workers on 2026-11-18; follow the current
[project-isolated worker runbook](README.md#project-isolated-workers). Do not
re-enable shared mode or move the old mixed-project volume into a private project.

## Consolidating the shared worker and sandbox controller (historical procedure)

The desired application namespace is now `harbormaster`. Agent Sandbox is included
in the main Kustomization; new `hm-prj-*` namespaces are application deployment
destinations. The shared agent remains one persistent workspace. Shared cluster
infrastructure, including Kata's host installation, keeps its existing ownership.

For an existing `workbench-reference` worker, applying the overlay alone is not a
storage migration. Use a coordinated cutover:

1. Publish and pin images from the consolidated-workspace implementation. The new
   runtime validates workspace-only policy selectors; old binaries reject them.
2. Archive the existing Sandbox/PVC/grant/policy identities privately, take a
   database backup and back up workspace data through the storage operator.
3. Stop the old sandbox controller before moving its ClusterRoleBinding to the
   service account in `harbormaster`. Apply the vendored controller with server-side
   apply, verify readiness and its new leader lease, then retire its old namespace.
   The controller binding's namespace change requires explicit field ownership
   adoption from the previous bootstrap manager.
4. Quiesce and stop the shared worker through the API. Stop controlplane and runtime
   writers during relocation. Verify the original PV has Retain reclaim policy.
5. Preserve the workspace's project/home/generation labels and matching Secrets.
   Release the stopped old claim and bind a new claim in `harbormaster` to that exact
   retained PV. Never reuse or erase another PV. Create the new Sandbox with matching
   ownership labels, the new pinned image and `harbormaster-workspace` PriorityClass.
   Remove stale owner references to old namespace object UIDs. Historical stopped
   workers can remain archived with retained PVs; shared mode does not restart them.
6. Apply the complete overlay. Core workspace policies select only workspace-labelled
   Pods, and its quota is PriorityClass-scoped. A namespace-wide one-Pod quota or
   default-deny policy would block the platform itself. Restart runtime/controlplane
   with the new binding and verify readiness, file hashes, PVC identity and gateway
   restrictions before deleting the old workspace namespace.
7. Create two disposable projects, verify distinct application namespaces and one
   shared worker, test application deployment using `serviceAccountName: application`,
   then remove acceptance workloads and their namespaces explicitly. Project metadata
   deletion intentionally retains application namespaces and their deployed data.

Never force-remove storage finalizers or delete the retained PV as a migration
shortcut. Rollback requires restoring the old binding, policies, Sandbox and claim
against the retained data; avoid simultaneous writers. Record the verified topology
in the reference README and commit the immutable image pins.

## Earlier two-core-namespace migration (historical procedure)

The desired core namespace is `harbormaster`, replacing `workbench-system` and
`workbench-egress`. Workspace namespaces, their
Sandboxes/PVCs and project identities remain separate. Kubernetes cannot rename a
namespace or move a namespaced object in place. Applying Kustomize alone does not
migrate existing state. The reference overlay also enables one shared workspace;
legacy workspace volumes remain stopped and retained.

The reference cluster completed this procedure on 2026-11-15; see the
[acceptance record](README.md#acceptance-and-recovery). Keep this procedure for
other installations still using the previous namespaces.

For an empty cluster, use the bootstrap and secret contract in the reference
README. For the existing reference installation, schedule a coordinated cutover:

1. Build/publish images from the namespace-consolidation commit or later. Pin the
   new runtime-controller and workspace digests, and rebuild optional Hermes images.
   Old runtime binaries enforce the old namespace selectors; old Hermes adapters
   reject the new internal endpoint. Do not deploy the changed policies with old
   binaries. Review retained-Sandbox image drift before restarting workspaces.
2. Stop workspace execution through the API and verify storage synchronization.
   Record the existing Sandbox/PVC identities, gateway Service IPs and application
   versions. Take a verified database backup and workspace-volume backups using
   the cluster operators. Keep the old database and namespaces available for rollback.
3. Create `harbormaster`. Provision matching application identities, scoped registry
   credentials and gateway configuration there through the secret manager. Preserve
   the existing signing keys and grant secret. Secrets are namespace-scoped; applying
   the overlay does not copy them.
4. Prepare the database in `harbormaster`. Use CNPG recovery, or initialize the
   target database and restore a PostgreSQL logical backup with the application
   owner. Update the database URL and CA in `controlplane-secrets`. A preliminary
   restore can validate the procedure while the old application continues serving.
5. Stop the old core Deployments, take a final consistent backup and restore it into
   the target database before starting the new controlplane. Verify project/workspace
   identities and counts. Release the three old gateway Services only after
   recording their specifications, then create their replacements in `harbormaster`
   with the same reserved IPs. A ClusterIP cannot be allocated twice. Keeping these
   IPs preserves existing workspace gateway destinations without opening DNS access.
6. Apply the new core manifests and workspace network policies/RoleBinding together.
   The RoleBinding remains in `workbench-reference` and refers to the core runtime
   ServiceAccount in `harbormaster`. Keep the namespace AND pod-label selectors in
   each network peer; permitting the whole core namespace would expose other services.
7. Reissue/provision TLS in the new namespace and switch the authenticated Ingress
   only when the new dashboard is ready. Remove the old Ingress to avoid duplicate
   hostname routes. Confirm Entra sign-in, authenticated API access, retained data,
   gateway access and rejection of workspace access to other core services.
8. Verify that new projects see the same `ws_shared` worker and agent home, and
   that start/stop preserves its files and PVC. Leave legacy Sandboxes suspended:
   their image pins intentionally differ and shared mode blocks their execution.
   Retire old core resources only after the rollback window. Do not delete old
   namespaces as a shortcut: they contain database and identity state.

If verification fails, stop the new core and restore the recorded Service/IP,
Ingress, RoleBinding and network-policy configuration before restarting the old
core against its retained database. Avoid concurrent writers; database changes
made after cutover require a deliberate recovery plan before rollback.

This is an operator procedure, not an automatic Argo CD migration. Argo adoption
remains manual with state pruning/deletion protection.
