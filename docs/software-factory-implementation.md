# Software factory implementation status

Updated 18 November 2026. Tracks delivery against
[the plan](software-factory-plan.md). The first development workflow is integrated:
work requests, approved workers, evidence, independent
retained previews, assistant handoffs, and operator UI. Production acceptance
remains gated on real infrastructure; the running deployment has not been updated.

## Work execution and review

Work items accept an inline instruction (up to 64 KiB) or reference a private
workspace specification and its SHA-256 digest. Omitting the worker provisions an
approved private Codex workspace through the shared creation/quota policy.
Inline instructions are published immutably to private storage; retry keys bind
the original bytes and worker. Central SQL retains references and fingerprints.
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

Startup migrations currently cover versions 1–8, including work records,
attempts, exchange grants/bindings, consumption receipts, immutable direct-request
receipts, and deletion tombstones. Existing workspace
ownership, homes, native histories, and schedules remain intact.

## Artifacts and retained environments

Workspaces publish immutable files through the [guest factory contract](workspace-factory-contract.md).
A project can promote an archived site into a separately registered workspace.
Promotion has durable retry receipts, and each retained allocation admits one
active service. Expiry and revocation reconcile through confirmed compute release.
Artifact list and read operations can wake stopped private storage. HTTP evidence
reads allow 1 MiB, and MCP reads allow 512 KiB to keep the SDK's duplicated output
inside the fixed broker response budget.
The demo recipe API and GoGallery workspace image were retired in #57. Artifacts
already published by those jobs remain readable through the artifact catalog.

Local browser attachment uses a distinct service-specific `.localhost` origin,
one-use tickets and host-only sessions. Every static fetch checks service state
and expiry through the authenticated platform route. It does not expose guest
ports or render guest HTML on the operator origin. Remote ingress and production
k3s browser attachment remain deployment gates.

Operator workspace/project deletion requires exact-ID confirmation and refreshes
runtime state before confirming an idle stop. Project deletion fences concurrent
creation and hides its workspaces. Tombstones preserve identities and receipts;
**private disks remain on the host**. Physical disk purge is not implemented.

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
host-only. The shared fixed tool policy includes work create/list/run/attempts/cancel,
exchange publish/inbox/read/submit, note publication, and factory
evidence and preview operations. Human grant administration
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
Kubernetes Hermes supports only an exact administrator-approved image digest and
configured model. Its fixed cluster MCP endpoint checks generation grants through
the trusted controller and derives project identity from sandbox metadata. See
[k3s Hermes setup](k3s-hermes.md). Matrix remains unsupported on this backend.
Live image acceptance, multi-user membership/authentication, real CNI/Kata
acceptance and recovery drills remain production gates.

The running Compose deployment still uses its installed images. Rebuild/restage
and perform the documented host acceptance before enabling new guest capabilities.
The Hermes MCP signing-key provisioning and image changes require the trusted
host installation path. Live Matrix/account/Obsidian connections and booted-VM
scheduled digest acceptance have not been established by the local tests.

## Verification

The integrated contributor checks passed: Go builds, full race suite,
architecture checks and vet; frontend formatting, 33 unit tests, typecheck and
production build; portable Python fixtures and real pinned Hermes SDK composition.
Browser acceptance additionally covers work requests, previews and revocation,
delete confirmation, assistant setup, profiles, and project switching. Final
recovery guards have targeted race coverage.

```sh
HERMES_SOURCE=/path/to/pinned/hermes GOFLAGS=-p=1 make check
make browser-check
HERMES_SOURCE=/path/to/pinned/hermes go test -race -p 1 ./internal/api ./internal/runtime/firecracker
```

Real pinned SDK composition tests exercise digest publication into a private guest
store and inbox consumption that creates one destination work item/attempt across
retries. They use real HTTP/MCP/workspace components with synthetic project data;
they do not claim Firecracker boot, provider login, PostgreSQL upgrade acceptance,
or k3s isolation verification.
