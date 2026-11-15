# Issue board

The project board has four columns: **Planned**, **In progress**, **Review** and
**Done**. Drag a card between columns or use its action menu to move it with a
keyboard or touch screen. Select a title to rename the issue, move it, open its
work thread or manage repository sync.

Failed, blocked, cancelled and unknown work is highlighted on its card. The
**Flagged** filter shows work and sync problems without adding another column.
Titles omit generated agent preambles. Search matches titles, source repositories,
issue numbers and work IDs.

## Standalone issues and Hermes

Use **New issue** to enter a title, description, and work type. No GitHub or
GitLab connection is needed. Creation saves a Planned draft using the existing
private workspace/specification flow; it does not dispatch an attempt. The title
and editable Markdown description are kept in project-scoped board metadata. The
initial request also remains in the private workspace as an execution snapshot. Retry
keys prevent duplicate issues and do not overwrite later board edits.

Hermes has three project-scoped tools: `board_list`, `board_create`, and
`board_update`. It can create standalone issues, edit titles and descriptions, move columns, and
archive or restore them. Updates require the current revision from `board_list`;
conflicts must be reread, not blindly retried. These tools do not sync to a remote
repository or approve reviews. Execution still uses the existing `work_run` tool.

The tool policy is embedded in the project MCP server and Firecracker host broker,
and installed in the Hermes guest image. Updating a running Firecracker deployment
requires both the host controller and guest image update described in
[Firecracker operations](firecracker.md#operations-and-limits). Updating only the
MCP container does not activate new tools in an existing guest. Pause Matrix,
stop idle workers through the API, install the staged image/controller, then
restore workers and Matrix; never replace the runtime beneath active guests.

## Planning, execution and archive

Board placement is durable planning state, separate from worker execution and
formal review acceptance. Dragging a card does not dispatch or cancel a task,
approve a delivery, or merge code. The details panel shows actual worker status
and links to work/review actions. Until a user changes placement, columns follow
execution state. Subsequent execution state changes update active placement;
archived cards remain archived. Accepted and Closed work belongs in Done unless
explicitly moved out. Drag it, use the card menu, or change Status in issue details
to reopen it in **Planned**, **In progress**, or **Review**. This updates the task
and board together, clears the current review decision, and retains the previous
decision in review history. In progress is planning status here: no agent starts
until **Start task** or a new conversation request. Moving reopened work between
active columns keeps its task status in sync; moving it to Done closes it without
approval. Returning it to Review makes the existing result reviewable again.

Archiving and restoring to Done preserve the existing approval or closure. Old
conflicting board metadata still resolves to Done until an explicit reopen move.
Repository refresh does not implicitly reopen accepted or closed work; use the
board move to choose that transition. Board revision and task-state checks make
reopening atomic and reject stale changes without partially changing status.

Use **Archive issue** to hide a card. The archive is a separate view, never a board
column, with a restore action that returns the card to its previous column.
Archiving does not stop active work or change its provider issue state. Local
unsynced changes made before archive are retained. Revision checks reject stale
moves rather than overwriting another session's changes.

Ticket titles, Markdown descriptions, columns, archive state and source snapshots
are stored in the project-authorized board table. The original execution
specification remains a private workspace file; editing a ticket does not rewrite
an already saved or executing work request. Use the work thread to send updated
instructions to a worker.

Ticket details follow a title-and-body layout, with status and repository controls
in a sidebar. Description editing has Write and Preview modes and supports GFM
headings, links, tables and task lists. HTML is not executed. Existing tickets
without a stored description display their private work request as a fallback;
saving the description makes it durable independently of the workspace. Clearing
a description is explicit and does not restore the old fallback.

## Selected repository issues and two-way sync

Use **Import issues**, choose a platform repository connection, and select the
issues to bring onto this project's board. Imports are individually retryable and
deduplicated by connection, issue number and destination project. Partial failures
show each failed issue and leave those issues selected for retry. Import creates
a planned factory work item and an enduring source reference; it does not run it.
Existing imports can be attached to their source without creating another work
item. A closed source issue starts in Done.

In issue details:

- **Sync to repository** sends the local title, description and open/closed status. Done maps
  to closed; Planned, In progress and Review map to open. The previous column is
  used for archived cards, so archive itself has no remote effect.
- **Refresh from repository** pulls the source title, description and status. Closed issues
  move to Done; reopened issues move from Done to Planned unless the factory task
  has already been accepted or closed. A repository refresh cannot undo that
  outcome; the difference remains visible as an unsynced status. Intermediate columns
  remain local. Archived issues remain hidden.
- Cards show **Not synced**, **Synced** (at the last sync), or **Sync failed**.
  No request runs implicitly when you drag a card or change its title.

Sync compares remote title/description/state with the last successful snapshot before writing.
If both sides changed, local edits are retained and no write is sent. **Use
repository version** explicitly discards local title/description/status edits to resolve a
conflict. Failed authentication, rate limits and removed connections leave local
changes intact. A retry whose remote values already match the desired result
updates the local receipt without another remote write.

Sync requires platform administrator authority. Project credentials can manage
local board state but cannot use platform credentials for remote writes.

This supports explicit title/description/status sync, not provider-native project board
columns, assignees, labels, Scrum sprints or webhooks. The
provider read/check/write sequence is not an atomic lock against remote editors;
a change made during that short interval cannot always be detected by these APIs.
Execution specifications remain snapshots; refreshing an issue title/description/status does
not rewrite a task instruction already being executed.

## Visual design

Preserve the existing dark/light theme and Geist typography. Use the existing
canvas (#0a0a0a), card (#171717), text (#fafafa), blue (#60a5fa), amber (#d4a74f)
and done green (#34b399). Colour indicates stage or attention, not decoration.

    Search / Flagged                         Archive / Import issues
    Planned        In progress       Review           Done
    compact card   compact card      compact card     compact card

Four horizontal lanes use content-sized cards with three-line title limits,
small source references and concise sync status. Empty lanes have a quiet drop
area rather than full-height outlined boxes. On narrow screens the lanes scroll
horizontally within the board. The card menu remains a touch/keyboard alternative
to dragging; details use a responsive side sheet. Avoid repeated state badges
when the column already supplies that information.

For older linked tickets without a description sync baseline, refresh from the
repository before editing the description. A push cannot silently overwrite an
unknown remote description; it reports a conflict if the descriptions differ.

## Refreshing Hermes after tool schema changes

The long-running Hermes Matrix listener caches MCP input and output schemas.
After deploying changes to existing tool fields (for example, adding a ticket
`description`), wait for its current response to finish, then stop and start the
Matrix listener using the project Hermes controls. Restarting only the MCP
container does not refresh the listener's cached schemas. An error such as
`Invalid structured content ... description was unexpected`, or a claim that
`board_update` only supports title and stage, indicates stale discovery.

No host-controller or guest-image update is needed for fields added to an
already allowed tool. Verify fresh discovery with
`integrations/hermes/tests/workbench_hermes_tests/live_discovery.py`: it checks the description input schema and
makes a read-only board call using the pinned Hermes MCP client.
