# Operate and release Harbormaster

## Supported deployment

Run one control-plane replica. Its lifecycle locks and quota reservations are
process-local; increasing replicas is not a supported availability strategy.
PostgreSQL holds platform metadata. Workspace volumes hold private SQLite state,
files and agent homes. Vault holds integration credentials. These are separate
recovery units; backing up PostgreSQL alone does not back up user work.

The default Compose deployment is a local development installation. Keep its
published ports bound to loopback. The local dashboard requires the operator password. Hosted deployments combine approved sign-in with project membership for project routes. See [deployment setup](deployment.md) and the [backend acceptance requirements](architecture.md#backend-support) before serving a shared installation.

## Release gates

[GitHub Actions](../.github/workflows/check.yml) runs Go module verification,
formatting, builds, vet and race tests, a PostgreSQL contract, Python contracts,
frontend formatting and unit tests, Chromium workflows, Compose validation and
Go, Dockerfile, Kubernetes and workflow linting. Tests use synthetic credentials
and data. Never add production secrets to CI.

Before a release:

1. Run `make check`, `make browser-check` and `docker compose config --quiet`.
   Run `make postgres-check` with `TEST_DATABASE_URL` pointing to disposable
   PostgreSQL 17. See [CONTRIBUTING](../CONTRIBUTING.md).
2. Run `make go-vuln-check` and review dependency findings. `npm audit` runs in
   `frontend/`; review build dependencies as well as shipped JavaScript. Do not
   apply forced major-version downgrades without compatibility checks.
3. Require a green GitHub Actions run on the exact commit being released. Review
   `git status --short` and `git diff --check`; ensure all required new source
   files are included and local configuration/secrets are excluded.
4. Build the deployment images on a disposable host with `make up`, then run
   `make smoke` in fixture mode. Record the commit, image digests, architecture,
   Compose version and selected runtime. Unit tests do not verify packaged
   native tools or real network/VM isolation.
5. Run the selected runtime's live acceptance and a restore rehearsal. Do not
   claim production readiness for experimental adapters based on simulated tests.

Push the version tag after the release checks pass. GitHub Actions scans the
release images, publishes version tags and attaches the image inventory and
digest-pinned Kubernetes manifests to the GitHub Release. Update environment
overlays through reviewed pull requests using the digests in the inventory.
See [container publishing](container-publishing.md). The workflow does not
deploy to any environment.

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

Issue #46 starts a new schema version 1 for control-plane metadata and guest
SQLite. Existing development, staging and reference deployments from before this
change require fresh databases. Startup rejects the old schemas with a recreation
error. Leave the old databases and their migration records intact for recovery.

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

Rerun `make bootstrap` before restarting an existing Compose deployment so
the local launcher and credential encryption key files are present. Direct
Compose invocations must set `DOCKER_SOCKET_GID` to the group of the Docker
socket when it differs from the default. The Docker controller listens on its
management interface and retains outbound access to workspace networks.

For OAuth2 Proxy deployments, set `OAUTH2_PROXY_ALLOWED_EMAILS` on the dashboard
to the comma-separated list of permitted identities. The proxy must return the
verified email in its auth response. On controlplane, set
`PROJECT_CREATOR_EMAILS` for human accounts approved to create projects.
`MAX_PROJECTS_PER_USER` defaults to five. Existing project membership and
admin-bearer recovery remain available.

Provision the same `DASHBOARD_READ_TOKEN` and
`DASHBOARD_HUMAN_SIGNING_KEY` on controlplane and the hosted dashboard. The
dashboard uses its snapshot credential and signs short-lived human claims.
Remove its old control token mount. Provision a separate `PROJECT_SIGNING_KEY`
on controlplane, MCP, and the optional Hermes MCP service. Reissue project tokens
after the signing-key change. A Firecracker host that serves project MCP also
needs the corresponding project signing key. Keep the admin token and credential
encryption key out of the hosted dashboard and MCP Secrets.

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
