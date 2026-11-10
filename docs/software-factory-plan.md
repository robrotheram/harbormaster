# Software factory and research platform delivery plan

Status: proposed implementation plan, 9 November 2026. Product requirements below
are confirmed; the delivery sequence and implementation choices are proposed.

Workbench will provide persistent, isolated projects and on-demand compute for
software development, experimental research, and personal planning. T3 desktop
remains the primary interface. Hermes can organise and initiate work through the
same authorised operations available to other clients.

Develop and demonstrate the workflows using Docker Compose. Deploy production on
k3s after validating the actual runtime, storage, networking, and access controls.
The first software project is GoGallery; the second execution workload is an
experimental Research project. A Personal project holds the user's planning
assistant and personal notes.

This plan replaces the rollout sequence and broad EKS/k3s target in
[the earlier architecture proposal](architecture-direction.md). It preserves
project isolation, private agent homes, native agent integration, and the existing
module boundaries. [Current architecture](architecture.md) and
[service contracts](contracts.md) describe implemented behaviour.

Implementation progress and the currently available API are tracked in
[implementation status](software-factory-implementation.md).

## Confirmed scope

- Docker Compose remains the development deployment; k3s is the production target.
- Desktop T3 is the primary interface for coding, reconnecting to work, and review.
- Work must continue without the desktop being connected. Persistent state belongs
  on the compute platform rather than depending on files on the desktop.
- Multiple projects can have independent agents, workspaces, and test environments.
- Each user has a personal planning assistant; project assistants retain separate
  project context. The initial development experience can remain single-operator.
- GoGallery is the first application for the software-factory workflow.
- Experimental research is an execution workload, including running experiments
  and retaining evidence, alongside news and paper discovery.
- Hermes skills and Obsidian integration should support reading and daily planning.
- Infrastructure management for unrelated systems is deferred.

Compose deployment and workspace isolation are separate choices. Preserve the
selected sandbox backend, including the existing Firecracker development path
where configured. Adding a service or notebook must not silently switch a
workspace to a weaker runtime or grant an agent host Docker/Kubernetes access.

## Outcomes that define success

**Software development:** from T3, request a GoGallery change with acceptance
criteria. The platform allocates a workspace, records the source revision, runs
checks, builds a sample gallery, and exposes an authenticated preview. The user
can disconnect, reconnect, inspect changes and evidence, and decide what to merge.

**Experimental research:** start from a hypothesis or paper, prepare a versioned
experiment, run a baseline and a variation, and retain inputs, parameters,
measurements, logs, and conclusions. Another run can repeat the experiment after
the original compute has stopped. Successful execution does not automatically
mean the hypothesis was supported.

**Personal planning:** the personal assistant prepares a sourced reading queue and
daily plan using personal notes and explicitly shared project summaries. It can
submit an authorised request to a project assistant without acquiring that
project's complete files, conversations, or memory.

## Existing foundation and gaps

Reuse the following rather than replacing them:

- Immutable project ownership, private workspace storage and agent homes, explicit
  same-project file handoffs, native T3 attachment, and project-bound MCP access.
- Durable task acceptance and schedule occurrences, cancellation, activity
  observation, quotas, and restart reconciliation.
- Separate application operations, runtime providers, native agent adapters, and
  HTTP/MCP transports, as described in [architecture](architecture.md).

The required extensions are concrete:

- Work requests currently lack a durable outcome and review lifecycle above a
  task/run. Scheduled runs are tied to an existing workspace and instruction path.
- Runtime templates advertise fixed resource limits; there is no complete
  environment recipe, managed application-service, or durable artifact catalog.
- Native CLI tasks currently have a ten-minute overall timeout. Long experiments
  and retained previews need explicit job/service supervision and cancellation.
- [Hermes](../deploy/hermes/README.md) currently has terminal, file, and memory
  tools in the supported Firecracker integration. Workbench MCP orchestration,
  Obsidian integration, and the proposed cross-project exchange are additional work.
- [Kubernetes](../deploy/kubernetes/README.md) is an experimental Agent Sandbox
  adapter. It advertises no supported connection-policy modes, so normal starts
  are rejected. Hermes and branch seeding are unsupported there. Its configured
  Kata runtime and storage assumptions need real k3s acceptance.
- The human launcher is currently a local single-operator surface. Remote
  production access and use by multiple people need authenticated identities,
  device sessions, and project membership enforcement.

## Shared design

### Work items and execution attempts

Introduce a **WorkItem** as the durable request for an outcome. It owns the project
binding, requester, references to instructions and acceptance criteria, status,
execution attempts, and output references. Its private specification can describe
a code change, a research experiment, or a research digest.

Use an initial work-item lifecycle of draft, queued, running, blocked,
awaiting-review, completed, failed, and cancelled. A blocked item records the
reason, such as capacity, missing input, or an expired integration. Completion is
an explicit acceptance decision or a configured acceptance rule; an agent's exit
code alone does not complete the requested outcome.

Reuse and extend the existing Run projection for an execution attempt, including
the native task/session reference and recipe/input revisions. Preserve existing
schedule and task consumers through additive migrations. Do not introduce a
second scheduler or an independent workflow engine for the first delivery.

Freeze instruction and input references when an attempt is accepted. A delivery
retry returns the same attempt; an intentional rerun gets a new attempt linked to
the same work item. Interrupted external actions are not automatically replayed.
Map native T3 and Hermes execution to their actual engine state, with one execution
owner per attempt. Prevent simultaneous writers to the same checkout; parallel
attempts use separate workspaces/checkouts.

Instructions, acceptance criteria, research notes, and results remain in
project-owned storage. The shared control plane persists operational metadata and
references, not a consolidated store of project prompts or assistant memories.

### Templates and environment instances

Introduce an approved, versioned **EnvironmentTemplate** describing the toolchain
image, supported engines, resource class, setup/run/check commands, declared
services, storage requirements, network requirements, and retention defaults.
Resolve repository-supplied commands as untrusted code executed inside the
sandbox; repository content cannot widen platform privileges or networking.

An **EnvironmentInstance** binds a template revision to a project and records its
runtime allocation, health, services, storage, and expiry. Distinguish three uses:

- Development workspaces retain code and agent history and can stop when idle.
- Test environments use reproducible inputs and normally expire after a run or
  review deadline. Their lifetime is independent of the coding session.
- Research environments retain notebooks and experiment definitions, with compute
  allocated for interactive sessions or bounded batch runs.

Initially support one sandbox per environment, containing its required processes.
A retained preview uses a separate environment that consumes a published artifact;
it does not depend on the coding workspace remaining alive. Keep service and
dependency declarations suitable for a later multi-service test
environment, but do not build a general Compose-to-Kubernetes translator or an
arbitrary deployment API. Supported backends implement the same validated contract and
report unsupported features before allocation.

Use supervised jobs and services with explicit owners, health, logs, deadlines,
and cancellation. Starting a detached shell process is not a service contract.
Active jobs and retained services participate in idle-stop decisions. After a
restart, reconcile actual processes and preserve evidence of interruption; service
restart policy must not implicitly rerun an experiment or repeat a publish action.

### Service access and artifacts

Expose declared HTTP services through an authenticated, project-bound connection
broker, including browser automation and notebook sessions. Define routing,
WebSocket support, per-service origins/session isolation, and revocation on
stop/expiry. An agent cannot select an arbitrary host target or discover another
project's services. For Firecracker, this requires an explicit bounded guest
channel; for k3s, it requires supported Service routing and enforced policy.

Introduce **Artifact** metadata for test reports, generated galleries, screenshots,
packages, notebooks, measurements, and experiment reports. Record the owning
project, producer attempt, source/input revisions, content digest, and retention
policy. Keep the bytes in durable project-owned storage, separate from disposable
compute. Publish output atomically before reporting an artifact as available.

Start with a project artifact repository behind authorised read/write operations;
choose its storage implementation in the first milestone. Native engines may
retain private logs in their homes, but accepted evidence must remain accessible
without keeping the original worker running. Expiring compute does not delete
retained evidence. Quotas account for retained data as well as active compute.

### Personal and project assistants

Create Personal, GoGallery, and Research as explicit project records; never infer
ownership or migrate legacy data from names. Personal is private to its user.
Each assistant has its own home, memory, sessions, integrations, and credentials.
One assistant may run persistently while its workers are short-lived.

Provide a narrow **ProjectExchange** for cross-project coordination. A human
configures which source and destination can exchange which operations: publish a
selected status summary, submit a work request, or return a selected result.
Authorise both ends independently, record provenance, and support revocation.
Store exchanged content in the relevant project stores; route only references
and operational state through the shared control plane.

This is a new, explicitly controlled exception to the current blanket denial of
cross-project transfers. Ordinary file handoffs, native links, and credentials
remain project-bound. Summaries themselves are exports and may contain sensitive
information; only selected fields/content covered by the exchange are released.
The personal assistant receives no universal project token or cluster authority.

Hermes invokes shared work/environment operations through scoped MCP tools.
Workbench remains the durable schedule owner; Hermes/native cron must not create
a second trigger for the same job. A schedule targets a versioned work
specification and can wake its execution workspace.

Integrations are separate capabilities from skills. Register a supported skill
version, its dependencies, network destinations, credentials, and permitted data
paths. For Obsidian, begin with a configured vault or subdirectory and explicit
note writes, source links, conflict handling, and write history. Do not mount the
whole desktop home or overwrite concurrent user edits. Project assistants do not
automatically inherit access to the personal vault.

Use occurrence/attempt identifiers for integration writes and reconcile uncertain
delivery before retrying. For a note, keep a publication key and expected content
revision so an acknowledgement loss cannot create duplicate notes or overwrite a
subsequent user edit. Do not promise exactly-once actions from a remote service
that cannot support idempotent acceptance or reconciliation.

### User experience

Keep native T3 as the coding interface. Project-scoped MCP operations request work
and environments, retrieve state and artifact references, and prepare connections.
Native pairing remains an attachment step; pairing itself does not provision a
workspace. Validate this flow against the pinned desktop/server versions.

Extend the Workbench UI only for platform responsibilities: project setup, queued
and blocked work, environment/service access, evidence and review, schedules,
integrations, and usage. Follow the existing frontend feature boundaries. A result
must show what ran, against which revision, what passed or failed, and how to
reopen the workspace or retained outputs.

## Initial environment recipes

### GoGallery

Use [robrotheram/gogallery](https://github.com/robrotheram/gogallery) as the first
application. Its [README](https://github.com/robrotheram/gogallery/blob/5f1bfd6a41afec65bc7ab791a14863180d944ee0/Readme.md)
describes static gallery generation and a Fyne desktop application. Its
[Makefile](https://github.com/robrotheram/gogallery/blob/5f1bfd6a41afec65bc7ab791a14863180d944ee0/Makefile) includes Go
builds, npm theme builds, and native desktop dependencies. Resolve and record an
exact source commit for each attempt rather than relying on a moving branch.
The planning reference is commit `5f1bfd6a41afec65bc7ab791a14863180d944ee0`,
observed on 9 November 2026; M0 may select a newer baseline explicitly.

The recipe must provide a pinned toolchain, an isolated checkout, a small licensed
or synthetic photo fixture, writable metadata, and separate generated output.
Include different image sizes/orientations and nested albums in the fixture.
Keep source photos immutable and external publishing credentials absent from
ordinary test runs.

The first workflow is request, implementation, checks, sample-gallery generation,
browser checks, retained preview, and review. Tests exercise generated pages and
image variants, navigation, representative responsive layouts, and broken assets.
Record build time and resource observations for later capacity sizing; do not set
performance claims without a baseline.

Desktop build and GUI behaviour are a distinct test path. First validate native
dependencies and headless build/test feasibility, then add a virtual display and
repeatable desktop interaction checks. A passing website check is not desktop
test coverage. Scope the first delivery to CLI/site generation and retain desktop
testing as a named follow-up milestone.

Default output is a reviewable change plus evidence. PR creation can be enabled
through a scoped repository integration. Automatic merge, publishing a gallery,
and sending photos to an AI service require separately configured policies and
are not acceptance requirements for the first workflow.

### Experimental research

Start with a CPU-based Python environment supporting scripts and an optional
authenticated notebook service. Record dependencies using a pinned environment
definition. GPU execution and distributed training remain later capabilities.

An experiment specification contains the question/hypothesis, source references,
code revision, dataset version/checksum, parameters, seeds where relevant,
resource/time budget, and intended measurements. Distinguish immutable inputs,
writable scratch space, and retained outputs. Large datasets use managed,
project-bound storage and explicit acquisition rather than being copied into
every agent home.

The proposed first fixture uses a seeded synthetic regression dataset, a baseline
linear model, and one regularisation variant. Record held-out error and execution
time; use repeatability tolerances for model outputs rather than requiring identical
timings. It exercises the workflow without a large download or external account.
Persist metrics and a report that distinguishes observations from interpretation.
Record tolerances and nondeterminism where exact reproducibility is impossible.
A failed or cancelled run retains diagnostic evidence and does not publish partial
results as a successful experiment.

Literature collection is a related scheduled workflow: collect from configured
sources, deduplicate, retain source/date references, prepare a reading queue, and
draft notes. Turning a paper into an executable experiment creates a work item
with an explicit budget and inputs; reading an article does not itself authorise
running arbitrary downloaded code.

## Delivery milestones

Milestones are ordered by dependency and finish with a user-visible demonstration.
Estimate implementation effort after the bounded investigations in M0.

### M0 Validate the contracts and workload assumptions

Resolve environment/service boundaries, artifact storage, work-item transitions,
and a minimal recipe format. Pin a GoGallery source revision and establish its
build/test/preview commands and native dependencies. Define the research fixture.
Inventory the selected Compose sandbox backend and the intended k3s runtime,
storage, CNI, ingress, registry, and identity constraints.

**Acceptance:** versioned example recipes and work specifications exist for both
workloads, with a recorded local execution baseline and a capability list for each
runtime. Choose the bounded service-access mechanism before implementing it.
Unknown production details remain explicit gates rather than assumed support.

### M1 Durable work and retained outputs

Implement additive WorkItem/Run/artifact metadata and private specification
storage. Connect existing native task execution and schedules to the new records.
Expose shared operations through HTTP, project MCP, and a minimal work list/result
view. Preserve existing tasks, histories, and schedule acceptance keys.

**Acceptance:** a submitted work item survives client disconnect and control-plane
restart; duplicate delivery creates one attempt; an intentional retry creates a
new attempt. Output can be retrieved after worker shutdown. Project B cannot
enumerate or read project A's work or artifacts. A failed output publication is
visible and cannot produce a completed work item.

### M2 Managed environments and the GoGallery workflow

Implement template admission, resource classes, environment instances, supervised
jobs/services, expiry, and authenticated service access. Deliver repository import
and isolated checkouts from a selected commit; do not depend on unsupported
cross-runtime workspace cloning. Integrate the GoGallery recipe and browser checks.

**Acceptance:** from desktop T3, make a change, run checks, generate the fixture
gallery, and open its preview. Disconnect while it runs and reconnect to the same
work. Stop the coding workspace while a separately retained preview remains
available. Expiry revokes preview access and releases confirmed compute while
keeping evidence. Failed checks lead to a visible failure/review state. Existing
sandbox restrictions remain enforced.

This is the first useful software-factory release in the Compose deployment.

### M3 Experimental research workflow

Add the research recipe, dataset/input manifests, parameterised attempts,
long-running job limits, optional notebook access, and experiment reports. Reuse
M1/M2 operations rather than adding a separate research scheduler or service layer.

**Acceptance:** run the baseline and variation, inspect their measurements, stop
compute, and repeat from recorded inputs. Reports and notebook files remain
available. Cancelling an experiment terminates its owned processes, releases
confirmed capacity, and retains diagnostics. A notebook session cannot access
GoGallery or Personal storage.

### M4 Hermes coordination and personal planning

Connect project Hermes assistants to scoped Workbench operations. Add the personal
assistant, ProjectExchange, integration/skill configuration, Obsidian writes, and
one scheduled research digest. Make permissions and run status consistent across
Hermes, T3, and the platform UI. Support the selected development runtime explicitly
rather than assuming Hermes works in every backend.

**Acceptance:** the personal assistant requests selected GoGallery/Research status,
creates an authorised project request, and prepares a daily note containing source
links. Project assistants execute through the same work lifecycle. Revoking an
exchange blocks subsequent reads/submissions. Unshared histories and memories
remain inaccessible. Concurrent note edits produce a visible conflict rather than
silent replacement. Schedule retries do not duplicate runs or note publication.

### M5 Complete and accept the k3s production path

Run a small k3s compatibility exercise after M2 to catch routing/storage assumptions
early; retain Compose as the normal development setup. Production release requires
the full gate below after both workloads and Hermes can run.

Package the platform for k3s and complete the existing runtime adapter, including
connection policy, service access, template/resource admission, Hermes, and
observability. Retain the current single control-plane owner until durable
multi-writer coordination is separately implemented. Project provisioning must
install the namespace/policy/storage bindings before compute is admitted.

Define human/device authentication and project membership before exposing remote
management. Start with fair allocation on existing nodes, capacity queues, and
project concurrency/CPU/memory/storage limits. Adding nodes is a separate cluster
capacity operation; no general infrastructure-management agent is needed.

**Acceptance:** repeat M1–M4 demonstrations on the chosen k3s configuration. Prove
project network/storage separation, runtime enforcement, session revocation, and
absence of workspace cluster-admin authority. Exercise pod/node loss, storage
reattachment with one writer, quota exhaustion, pending capacity, cancellation,
and unavailable dependencies. Interruptions must be visible without replaying
external side effects. Restore metadata, homes, and retained artifacts from backup
and demonstrate the documented recovery procedure.

### M6 Broaden coverage after the first production release

Add GoGallery desktop GUI tests and packaging, richer multi-service application
recipes, further compute sizes, and additional research tools based on observed
workloads. Introduce node autoscaling, warm capacity, GPU support, or distributed
experiments only when capacity and latency evidence justify them. Workspace data
migration and live process migration are separate features.

## Implementation boundaries and review units

Keep each implementation change reviewable with its own acceptance test. The
expected sequence is:

1. Example recipes, workload baselines, and protocol decisions from M0.
2. Metadata migrations and private work-specification storage.
3. Work submission, attempt projection, retry/cancellation, and scheduler binding.
4. Artifact publication, authorised retrieval, and retention.
5. Shared HTTP/MCP operations and minimal work/result UI.
6. Template/resource admission and environment lifecycle operations.
7. Managed jobs/services, connection broker, expiry, and backend capability tests.
8. GoGallery recipe, fixtures, browser checks, and desktop T3 demonstration.
9. Research recipe, input manifests, notebook access, and reproduction tests.
10. Hermes tools, scoped exchanges, Obsidian integration, and scheduled digest.
11. k3s adapter completion, deployment/authentication, and production acceptance.

Items 6–8 depend on the work/artifact foundation; research reuses them. Hermes
orchestration depends on shared work operations, and cross-project planning cannot
ship before ProjectExchange enforcement. k3s design checks start in M0, with real
compatibility validation after M2 and full acceptance at M5. Split larger items
further by observable behaviour during implementation.

Application operations own policy. Extend `internal/orchestration` for work and
attempt dispatch, `internal/workspacesvc` and a focused environment service where
needed for allocation, and `internal/runtime/protocol` for neutral capabilities.
Runtime implementations enforce those capabilities. `internal/api` and
`internal/mcpserver` adapt requests to the same operations. Preserve the existing
repository ownership of metadata SQL and native adapters' ownership of execution.
Do not add a message bus, generic workflow engine, or new agent loop without a
concrete requirement.

## Decisions to settle at the relevant milestone

- **M0, development runtime:** retain the configured sandbox; record which backend
  each demonstration uses. Service support must be implemented for that backend.
- **M0, storage:** choose durable project artifact storage and dataset handling;
  keep logical references independent of host paths. Select retention defaults
  before automatic cleanup is enabled.
- **M0, resource classes:** start with a small CPU-only set sized from the two
  fixtures. Persist actual limits and reject unsupported sizes. No GPU assumption.
- **Before M4, notes:** choose the Obsidian vault location and sync authority. Start
  with a bounded notes directory and conflict detection. Select the actual Hermes
  skills after checking their pinned requirements and permitted operations.
- **Before M4, exchange policy:** choose which project summaries and requests the
  personal assistant may exchange. Default to explicit human-configured bindings.
- **Before M5, cluster:** select k3s hosts, validated sandbox runtime, CNI/network
  enforcement, storage, ingress/TLS, registry, identities, and backup destination.
  The existing Kata/Agent Sandbox adapter is the starting implementation, not a
  verified production deployment.
- **Before M5, recovery and usage:** define acceptable recovery time/data loss,
  retention, concurrency/resource budgets, and provider account limits. Preserve
  the existing rule against silent paid-API or overage fallback.

These decisions do not block writing additive contracts or exercising small local
fixtures. Each becomes a delivery gate before its dependent feature is enabled.

## Verification and completion criteria

Use the existing [contributor checks](../CONTRIBUTING.md) for code changes, with
targeted invariants for ownership, lifecycle races, durable acceptance, schedule
recovery, cancellation, migrations, and capability negotiation. Add real
HTTP/MCP/controller composition coverage where interfaces change. Exercise
PostgreSQL upgrades as well as unit fixtures.

Run workload acceptance separately from simulated tests. Record source/template
revisions, deployment/runtime configuration, expected behaviour, actual evidence,
and unresolved failures. Compare supported runtime implementations against the same
behavioural contract while requiring runtime-specific enforcement evidence.
Unknown activity blocks unsafe stop; failed termination does not free capacity;
missing capabilities fail before allocation where possible.

The first production scope is complete when GoGallery development and Research
experiments work through desktop T3 on k3s, Hermes can coordinate only the granted
operations, evidence survives compute shutdown, and recovery/access controls pass
M5. Broader infrastructure management, arbitrary user-supplied deployments,
automatic production publishing/merging, GPUs, and a highly available control
plane remain outside that release.
