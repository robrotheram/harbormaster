# Workbench consolidation — implementation status

> Historical record. See the [documentation index](README.md) for current setup and workflows.

17 November 2026. This describes the current checkout, including pre-existing
uncommitted changes. Local dashboard/API and task execution were verified on 17 November; this is not a production deployment certification.

## Implemented

- Workspace, Reviews, Overview are the primary project destinations. The existing `/work` and `/team` route IDs remain
  unchanged so saved links keep working. Workspace remains the
  default project section. Existing `/board` URLs render Overview with the same work
  identity; `?work=` also opens the planning details.
- Overview uses existing board records, imports, descriptions, archive and restore
  actions. Open is the default and includes all unfinished tasks, including assigned
  planned work. Active, Planned and Completed scopes derive from outcome/run
  state rather than visual column placement. Creating a planned item does not dispatch it.
- Workspace and Reviews reuse explicit board titles. Derived private titles are cached
  only within the current mounted project/account. Opening a stopped worker's
  private instruction can still fail; that failure does not disable valid review
  actions. No instruction-derived text is newly stored in the control database.
- Review links carry work and attempt IDs. A newer run does not silently change
  the output being reviewed. Earlier accepted output remains viewable, without
  actionable approval controls. Workspace links back to the relevant attempt.
- Approval accepts an attempt and completes its work item; it does not integrate
  or deploy code. Expected revision is checked in the same metadata snapshot used
  by the database compare-and-swap update. Live application previews are labelled
  as live, not represented as immutable evidence of an older output.
- Conversation and review drafts stay in memory across project-route navigation,
  scoped to project and authenticated identity. Project changes, sign-out and
  reload discard private drafts. Last-thread selection is restored within that
  mounted project. Thread order does not move beneath the pointer on polling.
- Compact shell, 240–280px keyboard/pointer-resizable thread list, and closed,
  split, and focused output states use personal browser layout preferences.
  Preview readiness does not force the pane open. An explicit open preference
  survives application failure. Existing mobile switches and preview controls
  are reused.
- Owner labels, priority and dependency references extend existing board
  metadata. Writes use project authorization and revision checks. Dependencies
  must identify other work in the same project; missing, duplicate and self
  references are rejected. These are coordination references, not scheduler
  barriers. Imported issue sync continues to send only its existing fields.

- Project deletion uses explicit project-administrator membership, with confirmation,
  worker-stop checks and shared-worker retention preserved. Deletion clears the
  removed project’s cached queries and remembered navigation context.
- With no accessible projects, the sidebar contains only Projects navigation.
  Getting started offers Add sample project only while the project list is empty.
  It creates Sample project and three editable planned tasks in Overview atomically through
  the idempotent creation flow, then opens Overview. Tasks have descriptions, priorities
  and dependencies, with unassigned owners. They have no workers, instructions or
  attempts; they cannot dispatch until prepared for execution. Creation retries
  preserve edits and do not duplicate tasks. No reviews or completed runs are fabricated.

## Overview polish

Overview now sorts by priority by default, with named priority/activity pills,
consistent filter alignment and mobile task cards. Task details uses one editor
for title and description, preserving drafts on failed saves. Assign to me and a
searchable active-member picker persist the member ID plus the display email;
server writes reject non-members and inactive memberships. Legacy owner text is
still displayed, and can be replaced by a real assignment.

Migration 16 adds permanent per-project task numbers. The UI uses linked #number
references, dependency titles, a task picker and copyable task links. Existing
work IDs and URLs remain valid. References are allocated once and do not change
when sorting or editing tasks.

Create task now saves planning metadata without worker setup. Start work prepares
the current title/description on a worker and creates the first run on the same
task, claiming unassigned agent control for the initiating human. A stable first-run
receipt prevents retry duplication. It retains project membership and controller
checks; human execution remains blocked in shared-worker mode. The UI explains
that configuration requirement before starting.

Import and repository sync still use platform-global connections. Signed-in
project members see actionable operator guidance before any forbidden connection
request; access has not been broadened. Per-project repository grants would need
a separate permission model.

Validation includes backend reference/assignment/start/retry and shared-mode tests,
browser editing/assignment/dependency/import/start flows, a mobile overflow check,
and automated accessibility checks on the task pane. Screenshots were inspected
at desktop and mobile sizes. These are fixture tests, not live multi-user rollout.

## Audit findings and remaining delivery work

This is a partial implementation of the full redesign, not completion of its
multiplayer acceptance criteria.

Explicit membership is now implemented. AuthKit and the trusted OAuth proxy supply
verified human identity to short-lived signed API credentials. Login alone grants
no project membership. Administrators create email invitations; users accept them
from Projects. New project creators become administrators. Existing projects are
not automatically assigned to anyone. Local/system operator credentials remain
an explicit recovery boundary for bootstrapping existing projects.

The execution API checks active membership and the current controller. Workspace
Details exposes controller transfer and one optional required reviewer. Handoffs
record old/new controller, actor and time; active runs must stop first. Stale
transfers fail. Review decisions record the human reviewer and bind to the attempt.
Changes requested has its own state. A reviewer without control can record that
decision but cannot dispatch a follow-up or stop someone else's run. Private
conversation text remains on the worker.

Further implementation needs:

- Multiple required reviewers and recorded feedback for a non-controller review.
  This release supports one required reviewer, not quorum approval.
- Separate team comments from agent instructions only once an actual comment
  endpoint and audience contract exist. No cosmetic comment control was added.
- Define and test per-run code isolation and integration, including revision
  rechecks and actual conflicts. Current active-workspace serialization and run
  idempotency do not prove branch/worktree isolation or safe parallel integration.
- Add authenticated Mine/reviewer filtering, notification deep links, and stable
  navigation preferences across reloads. Legacy owner text is not a safe identity key; new assignments use active member IDs.
- Extend scroll restoration to semantic anchors across historical pagination;
  current restoration reapplies the saved position as asynchronous content arrives.
- Finish accessibility/reflow coverage and user-task timing for solo, team and
  reviewer flows. The existing legacy board drag tests need replacing with Overview
  coordination tests, not treating their removal as evidence of parity.

The repository-wide work list currently caps results at 200. Full searchable
history needs a paginated API contract before counts can claim completeness.

## Validation

Targeted API/orchestration/store tests cover project fencing, idempotency, stale
review and board revisions, coordination edits, and private-text boundaries.
Frontend unit checks cover state derivation and existing title/evidence logic.
The consolidation Playwright test covers readable titles with unavailable private
instructions, 1440px focus width, draft round trips, Overview-to-review navigation and
non-actionable historical approvals. The browser regression suite uses fixtures. A signed-in local browser check also
created a separate verification project, created and started a task, observed its
`READY` reply and Awaiting review state, and verified recovery of an earlier failed
attempt. Multi-user live integration was not exercised.

## Membership rollout

The local dashboard/API were rebuilt together during the 17 November repair.
For other deployments, deploy dashboard and API together and apply schema
migrations through 16 before enabling the new UI. Existing projects must receive an explicit
administrator invitation via the local/system operator's project settings or the
operator-authenticated `POST /v1/projects/{id}/members` API. Send `email`, `role:
"admin"`, `revision: 0`, and `action: "invite"` for a previously absent member.
Invitations are shown in-app; no email is sent. Invited people must also pass the
configured login allowlist. The OAuth authentication service must provide a
trusted email address alongside its stable user ID.

`SHARED_WORKSPACE_ID` is a single operator storage trust domain and cannot provide
per-project human isolation. Human execution/private-content routes fail closed
with `PROJECT_ISOLATED_WORKERS_REQUIRED` while it is configured. Provision and
verify project-isolated workers before rollout; do not silently move existing
shared-worker data into projects. Metadata/membership administration stays usable.

Browser console and application preview capabilities retain the issuing human
identity and recheck membership on subsequent API requests. Existing admitted
streams are not forcibly cancelled by membership revocation. Native desktop
pairing remains operator-only until its separate capabilities support human
membership revocation. Retire existing operator-issued capabilities during a
membership rollout. Global infrastructure/credential administration and the global
full snapshot remain operator-only; project administrator is not a platform operator.
Authenticated humans receive a service-only health summary with no worker inventory,
active tasks, global capacity or arbitrary diagnostics. Permission/configuration
blocks on worker and health queries stop automatic retries and polling; manual
refresh remains available. This prevents repeated alerts from persistent access
restrictions while preserving actual unknown/unhealthy service observations.

Membership tests cover invitation acceptance, other-user denial, revocation,
last-administrator protection, per-user creation retries, controller handoff,
assigned reviewer enforcement, signed identity tampering and shared-worker denial.
Browser tests cover invitation/revocation payloads and acceptance before discovery.

## 17 November load/start repair and UX verification

- The dashboard allowlist now exposes project capabilities and task starts while
  preserving signed human identity. Task starts no longer enter the worker-start
  URL rewrite. Board description writes use the API's existing 100 KB body bound.
- An unset model is omitted from task dispatch to preserve compatibility with
  installed workers that reject unknown fields. Explicit model selections remain
  immutable on their attempt and are still transmitted.
- The local Vault was sealed and its gateway token had expired. Existing recovery
  material restored Vault, and the gateway received a new renewable token using
  the existing read-only subscription policy. No provider login or permission
  policy was broadened. Provider credentials still have their normal expiry.
- All open tasks are discoverable by default. Legacy email assignments and member
  assignments share a single filter entry. Self assignments display You. Tasks
  without an agent run no longer claim a running agent. Access-check failures show
  a contextual retry notice and leave task details available. Closing task details
  restores keyboard focus to its opener, with a search fallback for deep links.
- Row actions share a fixed flex track. Geometry tests cover 1440, 1280, 1024 and
  390 px, including equal button heights and baselines and no horizontal overflow.
  Container queries switch to cards based on available content width.
- Verification: API/dashboard/orchestration/store/shared Go suites, 80 frontend
  unit tests, ten targeted browser scenarios, and automated contrast/semantics
  checks in both themes. Real ingress tests cover route registration, signed
  identity, descriptions above 4 KB, stable task identity and duplicate-start
  receipts. A real first-start task returned READY in its conversation.

Scope of the UX score: Overview and its task-details/start journey, not a claim
that every product surface or large-data/multi-user scenario has been validated.

### Final review rubric

Self-assessment of the verified Overview journey: **9.5/10**, up from **5/10** in
its error/disabled-start state. This is a design review, not independent user
research or a score for the whole application.

| Area | Score | Evidence |
| --- | --- | --- |
| Load and task execution | 2.5 / 2.5 | Signed-in loading, new task to READY, retry recovery |
| Task discovery and editing | 2.0 / 2.0 | Open scope, priority order, owner search, unified editor, linked references |
| Alignment and hierarchy | 1.9 / 2.0 | Consistent action geometry, restrained priority/activity pills |
| Responsive and keyboard use | 1.8 / 2.0 | Four viewport widths, mobile pane, focus return, automated accessibility checks |
| Feedback and state clarity | 1.3 / 1.5 | Scoped retry notice, truthful activity, pending labels, useful empty action |

Remaining evaluation limits: large datasets, multiple simultaneous humans, and
screen-reader usability need dedicated testing. The installed legacy worker still
uses its default model; listing alternative models requires compatible runtime
support. Vault sealing/provider expiry remain operational states, not UI fixes.

## Overview table refinement

Task ID has its own column; the Next step column and Details button are removed.
Clicking a row/card opens task details; the title remains a keyboard-operable button.
Start work, Review, Open thread and task-reference links keep their own actions.
Activity pills use distinct blue, amber, red, green and neutral states (plus violet
for Ready to start). Priority pills have consistent 12 px horizontal padding.
Dependencies remain available and editable inside task details.

## Thread direction and control

The thread header now leads with the task title and execution state. Worker,
creation date, assignee, controller and reviewer details live in a structured
Task settings side panel. Control changes are disabled during a run; membership
and server permissions remain authoritative.

The composer groups the instruction, compact model chooser and visible Send/Stop
action in one surface. Users can draft during execution without dispatching a
second run. Settings and model selection preserve the draft and restore keyboard
focus. Output focus controls sit in the preview header without overlapping its
reload/options toolbar. Personal output-layout choices remain respected.

Sending a new instruction after opening an output revision returns to the current
conversation, so subsequent activity and recovery actions remain visible.
Validation covers responsive layouts, draft retention, start/stop/retry, model
selection, review round trips, settings focus return and automated accessibility
checks. These checks use the browser fixtures; they do not certify all live worker
or multi-user scenarios.

## Unified task workspace

Conversation and Workspace share a single composer. Workspace contains Preview,
Files, and Settings. Opening Workspace hides the transcript while replies continue.
Preview shows live apps and generated charts or images. The former split/focus-output controls are removed.
Saved open-output preferences open Workspace, and the chosen view stays personal.

New assistant replies received while Workspace is open display a quiet message
notice and an unread indicator. View message returns to the latest conversation
without clearing the draft; incoming messages do not switch views or move focus.
Previously loaded messages are not announced as new. Stop and retry remain
available in Workspace. Preview selection drafts an instruction in the same view.
Charts and the application preview remain mounted when switching workspace tools.
