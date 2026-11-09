# Architecture and module ownership

This describes implemented boundaries. [Architecture direction](architecture-direction.md)
records the longer-term deployment proposal; the [original integration plan](implementation-contract-2026-11.md)
is historical. [Service contracts](contracts.md) describe wire behavior.

## Dependency direction

Application operations own policy and consumer interfaces. HTTP and MCP adapt
requests to those operations; concrete database, runtime, and native-process
adapters implement their dependencies. A feature's implementation should not
require changes to unrelated providers or screens.

| Module | Responsibility | Dependencies |
| --- | --- | --- |
| `internal/lifecycle` | Observation projection, idle-stop and connection-transition rules | Shared value types, standard library |
| `internal/workspacesvc` | Start/stop/availability operations, quota reservation, applied configuration | Consumer repository/runtime interfaces, lifecycle policy, neutral protocol |
| `internal/orchestration` | Schedule occurrence identity, dispatch/recovery, fair pending traversal | Consumer workspace/lifecycle/agent interfaces; its repository adapter owns scheduling SQL |
| `internal/api` | Authentication, project authorization, HTTP decoding/encoding and operation composition | Application services and metadata adapters |
| `internal/store` | Platform metadata, immutable ownership checks, profile revisions, startup migrations | SQL drivers and shared metadata types |
| `internal/runtime/protocol` | Provider-neutral request/capability types, generation grants, bounded workspace routing | Shared value types and standard library |
| `internal/runtime/docker` | Managed development containers, volumes and network policy | Docker API and neutral protocol |
| `internal/runtime/kubernetes` | Experimental pinned Agent Sandbox resources | Kubernetes API and neutral protocol |
| `internal/runtime/firecracker` | Host-owned jailed VMs, artifact checks and fixed guest channels | Linux host facilities and neutral protocol |
| `internal/workspace/filetools` | Confined filesystem operations | An owned `os.Root`, no supervisor or task database |
| `internal/workspace/taskstore` | Private task schema, recovery, durable acceptance and task persistence | Private SQLite and shared task metadata |
| `internal/workspace/nativeagent` | Native Codex, Claude and Hermes preparation, process lifetime and result protocols | Process fixtures and pinned provider tools; no database or HTTP server |
| `internal/workspace` | Supervisor lifetime, HTTP task interface, history/activity collection | Task storage, native adapters and filesystem tools |
| `internal/dashboard` | Local operator ingress, exact route allowlist, native attachment and frontend serving | Control-plane HTTP contract; no runtime or database authority |
| `internal/mcpserver` | Authenticated external-agent transport | Bounded control-plane client |
| `internal/gateway`, `internal/broker`, `internal/egress` | Approved model, operation and destination access | Trusted generation-grant verifier and deployment-owned configuration |

Provider implementations must not import one another. Application code uses
`runtime/protocol` and `runtime/controllerclient`, rather than importing a
provider through a shared runtime facade. Neutral rules belong in `protocol`;
provider enforcement belongs in the provider package.
Go architecture tests guard the application and protocol import boundaries.
Python Hermes adapters share fixed routing/configuration/framing helpers and
one pinned source manifest under `deploy/hermes/runtime.json`; portable adapter
contracts run without importing the real SDK or contacting a provider.

## Independent frontend features

`frontend/src/app` owns bootstrap, routes, and the application shell. Screens are
loaded by feature, rather than imported into the entry point. Feature folders
own their clients, queries, forms, and views: workspaces, tasks, scheduling,
history/activity, connection profiles, projects, project
assistants, and platform observations. Shared UI and HTTP utilities contain no
feature workflow. Import another feature's data API rather than its screen to
reuse a query.

The browser owns drafts, navigation, and presentation. Multi-step assistant
startup is a backend operation, so other clients can invoke the same policy.
Multiple candidate assistant workspaces produce an explicit conflict; the
operation does not silently choose one.
Connection-policy presentation helps explain the backend decision; it never
authorizes networking. Project-scoped query keys and response checks remain
mandatory. Private transcript/result queries have an explicit cache lifetime.

## Identity and content boundaries

- A project is the immutable ownership and authorization boundary. Display names,
  native thread names, and client-supplied IDs do not grant access.
- A workspace has stable identity and storage; its compute instance is replaceable.
  Each private home belongs to its workspace/project. Legacy empty home paths are
  preserved, not silently moved during startup.
- Instructions, results, native memory, and transcripts stay in private workspace
  storage. The control plane stores operational metadata and operator annotations.
- File handoffs authorize both ends, require the same project and idle sources,
  and publish without overwriting existing paths. They are explicit file copies,
  not implicit conversation or home sharing.
- Dashboard responses use an explicit allowlist DTO. Adding a workspace metadata
  field must not automatically publish it on the read-only listener.

## Coordination and recovery

The supported deployment has one control-plane owner. Per-workspace locks
serialize conflicting lifecycle operations, with stable lock ordering for
two-workspace operations. A separate short quota lock reserves metadata before
slow runtime startup; unrelated workspaces do not share a network-duration lock.
Do not remove those reservations or introduce a second control-plane replica
without durable claims/revisions and explicit multi-writer tests.

Observation is distinct from an explicit user-open. Task polling must not renew
the user's explicit-open timestamp. Cold task reads retain the existing ability
to wake private storage for access, without recording a user-open. Unknown activity blocks automatic
stop. Grant revocation and confirmed compute termination are separate facts;
failed termination must not appear as confirmed stopped capacity.

Scheduled delivery is at least once. An occurrence has a deterministic identity
and retries use the same durable workspace acceptance key. A lost acknowledgement
must not create a new task. Due occurrence insertion and schedule advancement
commit together. Pending traversal advances past unavailable or
long-running work instead of repeatedly selecting one permanent prefix. Missed
recurrences coalesce to one due occurrence, then advance to the next future time.
Restarting compute does not replay interrupted external side effects.

Metadata migrations run before listeners and workers. Initial idempotent versions
adopt the prototype schema without moving private content. New schema changes
must add a migration and an upgrade/reopen test; do not run DDL in handlers or
reconciliation ticks. Failed persistence transitions must be observable without
printing prompts, provider responses, tokens, or database credentials.

The control plane drains HTTP and joins its workers before closing metadata
storage. New long-lived components need an explicit lifetime owner and bounded
shutdown path.

## Backend support

| Backend | Status | Networking and acceptance |
| --- | --- | --- |
| Docker | Development implementation | Blocked, allowlist proxy, or explicit open networking; does not establish VM isolation |
| Kata via Docker | Operator-provisioned experiment | Requires a working shim; verified-isolation requirements fail closed without evidence |
| Kubernetes Agent Sandbox | Experimental adapter | No accepted network-policy implementation is advertised; normal startup rejects unsupported policy before allocation. Cluster policy verification/integration is required before enabling it |
| Firecracker | Host-installed sandbox implementation | No guest NIC; blocked/allowlist channels only. Requires root-owned pinned artifacts and real host/VM acceptance; no Docker fallback |

Capabilities are a typed, versioned contract decoded through the real runtime
client. The fixed approved template advertises CPU/memory limits. Workspace
metadata must match those limits before quota reservation or compute allocation;
the same limits travel in the authenticated launch request. Providers reject a
resource mismatch rather than silently applying different limits. Unsupported requested behavior must fail before allocating compute where
possible. A schema-compatible response does not prove that cluster/host isolation
is effective. Unit tests use simulated infrastructure; real acceptance remains
an operator-run deployment check.

## Tests and changes

Run [the contributor checks](../CONTRIBUTING.md). Tests should protect an invariant
or a boundary: project fencing, idle semantics, lost acknowledgements, bounded
queries, runtime capability compatibility, quota concurrency, termination errors,
schema upgrades, or browser workflows. Include real-client/controller composition
tests so matching mocks cannot conceal incompatible wire behavior.

Preserve distinct overflow behavior, cancellation, and unknown-state handling
when consolidating helpers. Remove compatibility code only after identifying its
remaining consumers and migration path. Avoid generic repository frameworks,
event buses, or one deployment per bounded context unless an actual requirement
justifies them.

Workspace tools are fixed: T3 and native Codex/Claude workers receive the bundled
file and Git MCP servers. There is no tool-profile catalog, editor, or workspace
selection. Hermes retains its own native toolsets. Old profile metadata is retained only to identify legacy workspaces;
it does not select tools or require an old profile record to start. Connection
profiles continue to control networking independently.
