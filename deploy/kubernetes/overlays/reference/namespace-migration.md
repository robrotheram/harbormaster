# Consolidating an existing installation

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
