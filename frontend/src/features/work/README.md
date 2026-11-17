# Work frontend contract

The Work section lives at /app/projects/:projectId/work. Existing shared request
transport prefixes paths with /ui-api; ingress must map /ui-api/work-items and
its subpaths to the corresponding /v1/work-items backend operations. The authenticated ingress maps these paths to the backend operations.

Lists and attempt lists return {items: [...]}; detail and create return a work
item directly. Mutations need a successful JSON response (an empty object is
sufficient for run/cancel/review). Every read carries project_id and every
mutation includes it. Attempts must belong to the requested work_item_id.
Work items must include project_id; mismatched responses fail closed.

Artifacts are objects with optional id, name, kind and content_url. Only returned
HTTP(S) content URLs or /v1/, /ui-api/, /dashboard-api/ paths are linked. There is
no constructed artifact host/path. No environment/jobs controls are implemented
until those contracts exist.

Action availability is centralized in workActions in api.ts for parent adaptation:
draft/failed/cancelled/rejected allow run; queued/running/blocked/pending allow
cancel; awaiting_review/review/review_required/pending_review allow review.
The backend stores `completed` when a human accepts a work item. Display it as
without an Accepted badge in the thread header or list, group it under Done,
and offer no further review actions. Attempt
execution state remains `awaiting_review`; the latest attempt's displayed outcome
uses its parent work item's acceptance. The backend remains authoritative for
transitions. Run delivery retries
retain their key until success; a subsequent intentional attempt gets a new key.
Creation retries retain the key for the same form payload.

The primary surface is a conversation, with work tracking beside it. A new
conversation saves a private instruction and requests its first run; if starting
fails, the saved draft remains selected and retryable. The advanced create form
also supports an existing specification path and digest. The selected conversation
is bookmarked with `?work=<id>`; switching projects clears it and all private views.

Recorded user/assistant messages and expandable tool calls/results come from the
worker's private history, filtered by the attempt's `task_id` (`episode`). Every
response must match the project, worker and task. Active runs poll for recorded
events; this is not token streaming or access to private model reasoning. Stopped
workers can be explicitly started to load their history. Private history and
output preview queries are discarded when their view unmounts.

`POST /v1/work-items/{id}/continue` takes `project_id`, `instruction`,
`expected_attempt_id` and `idempotency_key`. It saves an immutable private follow-up
file and queues a fresh native run in the same workspace, with a reference to the
previous request and the existing files. It does not inject into an active task or
resume a native session. Accepted, reviewable, failed and cancelled work can be
continued. The transaction snapshots the previous attempt's specification and
review outcome, preserving earlier outputs. Retries retain their key and recover
an accepted attempt or a lost private-publication acknowledgement.

Request changes focuses the conversation composer; sending that message queues
the revision. It does not silently reject the work. While an agent runs, users can
draft their next message or stop the run. Output selection can inspect any attempt;
only the latest reviewable attempt offers Accept result. Bounded text previews
escape content, never execute HTML, and fetch only authenticated ingress paths.
Attempt evidence and task IDs remain available under Technical details.

`GET /v1/work-items/{id}/instruction?project_id=...` reads and verifies a saved
specification for a draft. It does not wake stopped workers or store the text in
the control database.

Creation accepts `artifact_paths` (up to 32 private workspace files). Each attempt
publishes these paths with its own durable keys. Backend `/v1/` evidence URLs are
translated to `/ui-api/` in the browser so credentials remain in the ingress.
Review sends `expected_attempt_id`; a stale review cannot accept a newer attempt.

Application previews use project-authorized `/workspaces/{id}/applications` reads
and starts. The start payload holds a private application directory, shell command,
port, TTL and stable retry key. The worker owns the process group and publishes
observed readiness and bounded logs. Controls reuse service revocation and the
isolated preview connection endpoint. HTTP forwarding includes paths, queries,
methods and WebSocket upgrades without management credentials. Opening a conversation never starts an application.

The working view has a resizable conversation/preview split. History and retained
files are secondary controls rather than permanent stacked panels. `work-preview`
owns project-scoped app state, isolated embedded preview tickets, Web/Desktop tabs,
and selection-to-draft messages. Selection validates the frame window, exact
origin, and one-use nonce. `work-visuals` charts CSV/JSON through a bounded reader;
untrusted data never becomes markup. Layout falls back to a conversation/preview
switch below 1200px.

Port discovery polls `/workspaces/{id}/ports`. Eligible HTTP listeners are attached
through the existing application endpoint with `kind: forward` and a socket-bound
listener identity. Automatic receipt keys are stable across tabs. Previously
revoked listener identities are not automatically reattached. Collapsing preview
keeps the iframe and its authenticated connection mounted. Visualizations render
in the conversation rather than in a preview tab.

The UI supplies `presentation: "interactive-v1"` when creating inline work or
continuing it. The API adds chart guidance to the private instruction; the thread
shows only the user's request. `message-content` recognizes `workbench-chart`
fences and relative CSV/JSON links. `chart-spec` validates bounded declarative
settings and row data. `interactive-chart` loads `chart-engine` lazily, draws SVG
in the browser and provides exploration, table inspection, exports and drafts
about selected rows. Agent-provided settings are never passed directly to the
chart library. Missing values stay missing, and tooltips use rich text instead of
HTML. The manual Visualize data action uses this same renderer.

Linked data uses an immutable retained artifact when available, otherwise a
project/work/attempt-scoped POST to `/work-items/{id}/data`. Unretained files are
read only for the latest attempt. Private chart contents are not persisted in
browser storage. Inline chart data stays in the private conversation record.

Conversation list titles come from the first nonempty line of the saved
request, with generated presentation instructions removed. The same title is
used in the thread heading and list search. Saved requests use the existing
project-scoped, digest-checked private instruction endpoint and are cached only
while the view is mounted. Opening the list never starts a stopped worker. If
its request is unavailable, a specification filename or unique conversation ID
is shown instead of repeating a generic work-type label.

The list shows 20 threads at a time, with an explicit Show more action. Title
reads run in batches of four for displayed threads; searching scans all titles
within the selected status and reports progress and incomplete results. Failed
titles can be retried without starting a worker. Title cache keys include the
specification digest so follow-ups refresh their label, while list invalidation
does not reread unchanged private files. These caches also expire on unmount.
New-activity notices only announce newly recorded events, not loading older
messages. The message region supports keyboard scrolling and keeps the reader's
position until they choose to return to the latest activity.

Work fills the viewport below the application navigation. At 1200px and wider,
the thread panel stays visible beside the conversation and has no collapse action.
Its status filter uses a themed radio menu so its options remain readable in both
themes. Project sections live in the application sidebar, independent of this
Work-only panel. Smaller screens use the
Threads drawer from the conversation/preview navigation row. The rail and message feed scroll independently, leaving the
composer visible. Drafts are kept per thread (including a new thread) in the
current project's mounted view, and are discarded when leaving that project;
message text is never persisted to browser storage.

Work status labels use the task outcome, not its stage group. Both accepted and
closed tasks are grouped under Done, but closed work displays Closed in lists,
thread headers, reviews and board details.

Finished work can be reopened from the board. Manual Planned (`planned`) and
In progress (`in_progress`) states allow Start task or a follow-up request but
do not imply an active execution. Returning to Review clears the current decision
and preserves it in `work_review_history`. The attempt and its retained outputs
are reused until the user explicitly starts further work.

New threads and follow-up messages load their model choices from the selected
worker's `/models` endpoint. The worker filters the gateway's configured,
authorized model catalog by installed agents. The composer retains the current
thread model unless the user selects another; selecting the worker default uses
the worker's advertised configuration. There are no custom model ID fields or
browser-wide overrides. The separate Create work item entry is removed from the
thread sidebar.

Follow-up requests record an immutable provider/model selection on the attempt.
They retain the same work item and worker, with private context files containing
the previous request and reply and links to earlier turns. Conversation bodies
remain on the worker; the control plane stores references only. This transfers
conversation context across native agent invocations, rather than resuming a
provider-specific hidden session. Updated gateway, control plane, dashboard and
worker binaries must be deployed together for model discovery and context capture.

The thread Workspace menu groups Preview, Files, and Visualize data.

The thread header links to its assigned worker, using the `worker` search
parameter to filter the Workers view. Files also lists image and data links from
the latest agent reply, with inspection/download through the existing scoped
output endpoints. Visualize data uses chart blocks and linked CSV/JSON from that
reply before offering paste input. These private sources are kept in memory only.
Expired application previews can restart their saved launch configuration.

## Consolidated project surfaces

Work, Reviews and Team share explicit planning titles and private in-memory title
fallbacks. `?work=<id>&attempt=<id>` identifies a review revision; the conversation
can open that attempt. Review actions always target the selected current attempt.
Approval completes the work item, without merging or deploying. The repository
checks the expected attempt inside the compare-and-swap snapshot.

The project shell now retains drafts and title/reading context through route
changes in memory, and discards them when the project or authenticated identity
changes. This supersedes the earlier mounted-view-only draft lifetime above.
The shell defaults compact, the thread rail is resizable from 240 to 280px, and
output defaults closed. Layout preferences persist without private text.
Readiness never automatically expands output. Team replaces the board default;
legacy board URLs and planning/import/archive operations remain available.
See `docs/workbench-consolidation.md` for verified gaps and remaining multiplayer
work. Owner labels are planning metadata, not authorization or agent control.

Explicit project membership is now required for authenticated humans. Project
settings supplies invitations and revocation; Projects supplies acceptance.
Work Details assigns a controller and one optional required reviewer using active
membership IDs. Controller handoffs are recorded and require an idle work item.
The API enforces controller-only run/cancel/continue and assigned-reviewer decisions.
Review decisions record `reviewed_by`; `changes_requested` is distinct from run
failure. A reviewer who does not control the agent records a decision without
starting a run. Local/system operator access remains the recovery boundary.
Shared-worker mode is incompatible with per-project human isolation and rejects
human execution/private-output access until isolated workers are provisioned.
See the rollout section in `docs/workbench-consolidation.md`.
