# Operating and releasing Agent Workbench

## Supported deployment

Run one control-plane replica. Its lifecycle locks and quota reservations are
process-local; increasing replicas is not a supported availability strategy.
PostgreSQL holds platform metadata. Workspace volumes hold private SQLite state,
files and agent homes. Vault holds integration credentials. These are separate
recovery units; backing up PostgreSQL alone does not back up user work.

The default Compose deployment is a local development installation. Keep its
published ports bound to loopback. AuthKit restricts access to a shared operator
console, not isolated per-user projects. Consult [security](security.md) and the
selected [backend's acceptance requirements](architecture.md#backend-support)
before exposing or deploying it on a shared host.

## Release gates

The GitLab pipeline in `.gitlab-ci.yml` runs Go formatting/build/vet/race tests,
a real PostgreSQL contract, portable Python contracts, frontend formatting/unit
checks, Chromium workflows and Compose validation. It requires Linux container
runners with outbound dependency access. No Docker socket or privileged runner
is needed for these checks. Go race tests need the C compiler in the Go image.
Browser traces are retained for seven days; Go coverage for fourteen days.
Tests use synthetic credentials and data. Never add production secrets to CI.

Before a release:

1. Run `make check`, `make browser-check` and `docker compose config --quiet`.
   Run `make postgres-check` with `TEST_DATABASE_URL` pointing to disposable
   PostgreSQL 17. See [CONTRIBUTING](../CONTRIBUTING.md).
2. Run `make go-vuln-check` and review dependency findings. `npm audit` runs in
   `frontend/`; review build dependencies as well as shipped JavaScript. Do not
   apply forced major-version downgrades without compatibility checks.
3. Require a green GitLab pipeline on the exact commit being released. Review
   `git status --short` and `git diff --check`; ensure all required new source
   files are included and local configuration/secrets are excluded.
4. Build the deployment images on a disposable host with `make up`, then run
   `make smoke` in fixture mode. Record the commit, image digests, architecture,
   Compose version and selected runtime. Unit tests do not verify packaged
   native tools or real network/VM isolation.
5. Run the selected runtime's live acceptance and a restore rehearsal. Do not
   claim production readiness for experimental adapters based on simulated tests.

The pipeline verifies source; it does not publish images or deploy automatically.
Use immutable release image tags/digests in deployment overrides. The default
`:dev` workspace image is for local development. Base image tags and remote
package repositories can change, so retain the actual image digests used.

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
