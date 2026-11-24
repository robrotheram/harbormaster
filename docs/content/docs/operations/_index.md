+++
title = "Operate and release Harbormaster"
linkTitle = "Operations"
+++

## Supported deployment

Run one control-plane replica. Its lifecycle locks and quota reservations are
process-local; increasing replicas is not a supported availability strategy.
PostgreSQL holds platform metadata. Workspace volumes hold private SQLite state,
files and agent homes. Vault holds integration credentials. These are separate
recovery units; backing up PostgreSQL alone does not back up user work.

The default Compose deployment is a local development installation. Keep its
published ports bound to loopback. The local dashboard requires the operator password. Hosted deployments combine approved sign-in with project membership for project routes. See [deployment setup](../deployment.md) and the [backend acceptance requirements](../architecture/_index.md#backend-support) before serving a shared installation.

## Release gates

[GitHub Actions](../../../../.github/workflows/check.yml) runs Go module verification,
formatting, builds, vet and race tests, a PostgreSQL contract, Python contracts,
frontend formatting and unit tests, Chromium workflows, Compose validation and
Go, Dockerfile, Kubernetes and workflow linting. Tests use synthetic credentials
and data. Never add production secrets to CI.

Before a release:

1. Run `make check`, `make browser-check` and `docker compose config --quiet`.
   Run `make postgres-check` with `TEST_DATABASE_URL` pointing to disposable
   PostgreSQL 17. See [CONTRIBUTING](../development/contributing.md).
2. Run `make go-vuln-check` from the repository root and `npm audit` from `frontend/`. Review findings in build dependencies and shipped code.
3. Require a green GitHub Actions run on the exact commit being released. Review
   `git status --short` and `git diff --check`; ensure all required new source
   files are included and local configuration/secrets are excluded.
4. Build the deployment images on a disposable host with `make up`, then run
   `make smoke` in fixture mode. Record the commit, image digests, architecture,
   Compose version and selected runtime. Unit tests do not verify packaged
   native tools or real network/VM isolation.
5. Run the selected runtime's live acceptance and a restore rehearsal. Do not
   claim production readiness for experimental adapters based on simulated tests.

After the checks pass, follow [container publishing](../container-publishing.md#releasing-and-deploying) to tag the release and update deployment overlays.

## Upgrade and recovery

Before changing versions, stop accepting new work and stop workspaces through
the workbench. Confirm active tasks have finished or were explicitly cancelled.
Record workspace IDs, volume associations, runtime configuration and the current
release's image digests. Stop the control plane before taking the final metadata
backup so scheduling/lifecycle writes cannot race the recovery checkpoint.

Take a PostgreSQL logical backup with `pg_dump` and verify it can be restored to
an isolated PostgreSQL instance. Snapshot stopped workspace volumes and Vault
storage using the selected runtime/storage driver's supported procedure. Keep
`.local/secrets`, Vault unseal material and deployment configuration in encrypted,
access-controlled backup storage, separately from source control. Treat all
backups as sensitive. Do not copy live SQLite database files without a consistent
SQLite backup or a stopped workspace.

Migrations run at control-plane startup. There is no general down-migration
contract. If an upgrade fails, retain failed-state diagnostics, stop the new
version and restore the matching pre-upgrade metadata/volume checkpoint with the
previous images. Do not run an old binary against an unverified newer schema.
Restoring state does not undo external side effects such as commits or provider
requests; reconcile them before resuming work. Never use
`docker compose down --volumes` as an upgrade or troubleshooting step.

### Pre-release database recreation

If startup reports an incompatible pre-release schema, create fresh metadata and guest databases. Keep the old databases and their migration records for recovery.

1. Stop the deployment and take the matching backups described above. Retain the
   previous images and deployment configuration so you can restore that checkpoint.
2. Provision an empty metadata database and point the control plane's
   `DATABASE_URL` or `DATABASE_URL_FILE` at it. For a SQLite deployment, use a new
   file path. For Compose or Kubernetes PostgreSQL, provision a fresh database
   and update the deployment's database URL secret.
3. Start the new deployment and recreate workspaces using fresh guest storage.
   The supervisor creates `homes/<home-id>/workspace.db` under the workspace
   data directory. Export project files you need to retain and import them into
   the new workspaces. The old guest database
   holds task receipts, history, artifacts and retained-service records. Keep it
   with its original backup and images.
4. Verify workspace creation and recreate the remaining platform metadata the
   deployment needs, including schedules and integration bindings. Preserve
   Vault and unrelated volumes.

## Security release upgrade

Run `make bootstrap` before restarting Compose. Bootstrap creates the local dashboard read and operator credentials if they are missing. Set `DOCKER_SOCKET_GID` for direct Compose invocations if the Docker socket uses a different group.

If the database contains credentials encrypted with the former control token, rotate `CONTROL_TOKEN`. Supply the previous value to the control plane as `CREDENTIAL_LEGACY_DECRYPTION_KEY_FILE`. Keep the active encryption key configured. Re-save the affected integration credentials before removing the legacy key.

Configure scoped credentials before restarting a hosted dashboard. `MAX_PROJECTS_PER_USER` defaults to five. `MAX_RUNNING_WORKSPACES_PER_USER` defaults to two. The global worker limit also applies. Reissue project credentials after rotating the project signing key. [Hosted sign-in](../authkit.md) · [Firecracker Hermes signing key](hermes.md#fixed-project-mcp-channel)

Hosted native desktop pairing is disabled. Browser-based worker consoles and
previews continue to use workspace connection tickets.

For an unresponsive guest, an administrator can call
`POST /v1/workspaces/{id}/force-stop` with a normal idempotency key. This
bypasses guest quiescing and preserves the workspace volume. Processes and
unfinished tasks can be interrupted.

## Health, shutdown and diagnostics

Start with `docker compose ps` and `docker compose logs --since=15m --tail=200`.
`/healthz` is service-specific liveness/readiness; a healthy process does not
establish upstream login validity, successful agent execution or isolation.
Check workspace activity and the selected runtime when a task cannot progress.
Vault may be healthy while sealed: its Compose probe intentionally accepts that
state. Unseal/provision it before enabling real integrations.

Standalone HTTP services handle SIGINT/SIGTERM, drain active requests for ten
seconds and then cancel remaining request contexts. Compose allows thirty
seconds before forcible termination. Handlers must honor cancellation; upgraded
connections and external processes have their own lifecycle requirements.
A forced stop can interrupt work; inspect retained task state on restart rather
than automatically replaying external actions.

For support, record the release commit/image digests, backend, service health,
time window (UTC), opaque project/workspace/task IDs, operation and safe error
code, plus relevant sanitized logs. Do not attach `.env`, secret files, database
URLs, authorization headers, private transcripts, raw backups or browser traces
from a real operator session to public issues. Rotate any accidentally disclosed
credential; deleting an issue does not revoke it.
