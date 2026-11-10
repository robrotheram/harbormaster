# Software factory implementation status

Updated 10 November 2026. Tracks delivery against
[the plan](software-factory-plan.md). Implementation is in progress: GoGallery
admission/retained previews and direct instruction/worker provisioning are being
completed separately. Production acceptance remains gated on real infrastructure.

## Work execution and review

Work items reference a private workspace specification and its SHA-256 digest.
Creation stores a draft. Running creates a durable attempt with a stable delivery
key; the existing scheduler dispatches through native workspace acceptance.
The guest verifies specification bytes on first acceptance and retains receipts
across restart. Retry recovery resolves the same receipt even while its task is
busy. New file-based tasks cannot compete with active workspace jobs.

Project-scoped HTTP and MCP support creating/listing work, running attempts,
listing attempts, cancelling, and explicit review. The Work UI exposes these
operations using project-bound queries. Review carries `expected_attempt_id` so a
stale result cannot accept a newer attempt. Cancellation intent survives uncertain
dispatch and subsequent busy/rate-limit responses.

Creation accepts up to 32 `artifact_paths`. Each attempt publishes those files
with its own durable keys before reaching review. Immutable bytes and digests
remain in private workspace SQLite storage; central metadata holds references.
Artifacts survive compute stop. A cold download wakes private storage through
the runtime without replaying the task. Browser downloads use authenticated
`/ui-api` routes and attachment responses.

Startup migrations currently cover versions 1–6, including work records,
attempts, exchange grants/bindings, and consumption receipts. Existing workspace
ownership, homes, native histories, and schedules remain intact.

## Research jobs

The guest admits the built-in `research-v1` recipe through `/v1/recipe-jobs`.
It executes the real standard-library regression baseline and regularisation
variation, records measurements in `outputs/{job-id}/`, and supports status and
process-group cancellation. Jobs participate in activity/idle decisions and
restart recovery records interruption rather than replaying execution.

The process profile uses a bounded deadline, address-space/process/file limits,
and requires Python and `prlimit`. It does not allocate a dedicated CPU core.
Guest artifacts can retain the metrics and execution log independently of the
source files. [Runtime contracts](../examples/factory/runtime-contract.md)
describe the exact guest endpoints and bounds.

The [example workloads](../examples/factory/README.md) include versioned work
specifications and recipe proposals. Their JSON files do not themselves authorise
runtime images, privileges, mounts, or networking. The pinned GoGallery source
built and passed its tests on the development host, and a synthetic nested-album
gallery built successfully using absolute fixture paths. Managed preview
acceptance is a separate requirement.

## Personal and project assistants

[Assistant handoffs](assistant-handoffs.md) implement explicit bilateral grants,
revocation, selected exports, a paginated destination inbox, and durable work
consumption. Retries recover the same work item and attempt. Central SQL contains
provenance/references rather than export bodies. Exchange locks are scoped to one
grant so a slow project store does not block unrelated exchanges.

An operator binds a project integration workspace through
`POST /v1/projects/{id}/coordination`. Export bodies and bounded note publications
are stored through authenticated guest operations, without host project mounts.
Notes use expected digests and durable occurrence receipts to detect conflicting
edits and deduplicate retries. External editors must honour the documented lock
for atomic compare-and-write; other edits receive optimistic digest checks.
The notes directory is project-owned storage, not an automatically connected
personal Obsidian vault.

Hermes has a fixed generation-authenticated Firecracker MCP channel. The host
issues short-lived project credentials per request; the signing authority stays
host-only. The narrow toolset includes work create/list/run/attempts/cancel,
exchange publish/inbox/read/submit, and note publication. Human grant administration
and work review remain outside that toolset.

The deterministic digest and destination consumer commands use Workbench-owned
schedules. Hermes schedules are accepted only for Hermes workspaces. Native tasks
receive `WORKBENCH_TASK_ID`, giving recurring digests distinct durable publication
keys while retries reuse the original task identity. No second Hermes cron is
installed. [Hermes setup](../deploy/hermes/README.md) describes broker provisioning,
image extras, and schedule fixtures.

## k3s path and deployment gates

The Kubernetes adapter now persists blocked/allowlist policies, checks destination
ceilings and NetworkPolicy drift, and verifies retained PVC ownership and RWOP
configuration. [k3s operations](k3s.md) include deployment rendering and
backup/recovery/acceptance scripts. Runtime status remains unverified and required
verified-isolation starts remain blocked until real enforcement evidence exists.
Kubernetes Hermes support, multi-user membership/authentication, real CNI/Kata
acceptance and recovery drills remain production gates.

The running Compose deployment still uses its installed images. Rebuild/restage
and perform the documented host acceptance before enabling new guest capabilities.
The Hermes MCP signing-key provisioning and image changes require the trusted
host installation path. Live Matrix/account/Obsidian connections and booted-VM
scheduled digest acceptance have not been established by the local tests.

## Verification

The combined contributor checks passed through the scheduled-Hermes integration,
including Go builds/race tests/architecture checks/vet, frontend checks/build,
and portable Python fixtures. Later inbox and recovery changes have targeted
race and real-SDK composition coverage; rerun the full checks after the remaining
implementations are integrated.

```sh
GOFLAGS=-p=1 make check
HERMES_SOURCE=/path/to/pinned/hermes go test -race -p 1 ./internal/api ./internal/runtime/firecracker
```

Real pinned SDK composition tests exercise digest publication into a private guest
store and inbox consumption that creates one destination work item/attempt across
retries. They use real HTTP/MCP/workspace components with synthetic project data;
they do not claim Firecracker boot, provider login, PostgreSQL upgrade acceptance,
or k3s isolation verification.
