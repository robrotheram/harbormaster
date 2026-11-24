+++
title = "Agent history and evaluation export"
+++

Open the project's **Settings** and select **Audit** to inspect recorded events
across the project. Filter by worker or event type. Expand an event to read its
content and see the model, tool, MCP server, and call ID when the adapter
records them. Audit history reads from the control-plane
database. In shared-worker mode it includes episodes linked to a work attempt
in the selected project and is available to the local operator.

In **Workers**, select **Open worker**, then **Activity**, then **Message history** to browse
its messages and download **Export JSONL**. Exports include tool calls, results,
lifecycle events, delivery echoes, and metadata. An export includes all records in its snapshot. Stopped and
unreachable workers return the last synced snapshot. The view shows when it
last synced. Start a worker explicitly to backfill older history that has yet
to sync.

Validation runs can carry a small **Test run** label on their messages and replies.
This uses an operator annotation attached to the episode ID; matching words in
an ordinary conversation does not classify it as a test. Exports include
`record.purpose: "test"` on all records of a marked run, so these runs can be
excluded from training. A real model may have generated a test run; this label
is separate from the `synthetic` fixture flag. The original transcript is retained.

History records user messages, visible final assistant replies, tool calls and
results, task outcomes, timestamps, episode/session/message/tool IDs, provider
and model where known, elapsed time, and available token usage. Hermes token
counts are reported as session-cumulative; Codex reports turn usage. Missing
counts mean unavailable, not zero. Do not sum session-cumulative samples as
independent per-turn counts. Historical task imports do not infer their model
from today's workspace settings. `at` is source event time where available;
`captured_at` is the recorder time.

- Native Hermes workspace tasks and Matrix replies include tool arguments and
  outputs. Matrix associates the approved room, requester and bot identity with
  each episode. `assistant.message` means a prepared response; a separate
  `message.delivery` records a successful/failed send and the returned event ID.
  A send acknowledgement does not prove the recipient read the message.
- Native Codex tasks include terminal/MCP/file-change interactions, final reply
  and available turn usage. Claude task workers currently include input, final
  reply and lifecycle, without intermediate tool detail.
- T3's local projected transcript is imported through its authenticated native
  API, restricted to the current workspace project root. Local user/assistant
  messages, commands, dynamic tools and file changes are included. Streaming
  replies, inherited messages, private reasoning, system notices, secret
  requests, provider payloads and compaction summaries are excluded. Native
  run IDs can be absent; retain session/message IDs when joining such records.
  Tool calls/results share a call ID. Oversized/unavailable native transcripts
  produce a `capture.gap`; transcripts over 8 MiB are not imported.
- Existing durable task prompts/results/tool messages are imported with
  `backfilled: true` and `capture_scope: backfill_task`. T3 records use
  `capture_scope: t3_projection`. These imports cannot recover data their
  original runtime never stored. Matrix records come from the running adapter. Earlier Matrix conversations and delivery receipts are unavailable.

The control plane copies redacted history from ready workers in batches during
reconciliation and attempts a final sync before a normal stop. Reads use the
cached copy when the worker is unavailable. A crash or force stop can leave
recent records unsynced. History from earlier installs backfills as each worker
comes online. The worker recorder remains the source for the cache.

Both database copies contain private project data. Apply your deployment's
access and backup controls to the control-plane database. Cached history stays
available in project audit history after a worker is deleted. Project deletion
hides that project's history through the API. Cached records have no automatic
expiry. Feedback and test labels require the operator credential. An export
attaches the latest rating per episode as `operator_feedback` with reviewer
`local-operator`. Unreviewed episodes have no rating. Audit records reflect what
the worker observed. Use them for review and debugging with the recorder's
coverage and integrity limits in mind.

Known credential formats, credential fields and the workspace grant are
redacted before recording. Structured JSON tool payloads remain JSON after
redaction unless truncated. This is best-effort redaction, not anonymisation:
messages and tool output can contain sensitive project data. Review exports
before sharing or training. Hidden reasoning and system prompts are excluded;
there is no automatic model training or upload.

History is append-only within a 256 MiB recording budget per workspace, separate
from the small rolling activity log. When full, new capture pauses and
`capture_limited` is exposed in the UI/export; existing history is retained.
Each record's content is limited to 32 KiB (UTF-8 safe); `truncated` and `redacted`
flags distinguish transformed data. Native frames over 1 MiB or malformed
frames cannot be parsed. Capture is best-effort and may miss intermediate output
around crashes, interrupted runs or unsupported adapters. Interrupted episodes
may lack a terminal record. Workspace disk loss can make unsynced history unavailable.

## JSONL contract (schema version 1)

`GET /v1/workspaces/{id}/history` returns a private page (up to 20 records),
cursors, recording-budget state and operator ratings. `order=desc&before=N`
pages backward; `after=N&until=H` pages forward within a fixed snapshot.
`POST /v1/workspaces/{id}/history/feedback` accepts
`{"episode_id":"…","rating":1}` from the operator.

`GET /v1/workspaces/{id}/history/export` streams an attachment:

1. `manifest`: schema/export/workspace/project IDs, export time, fixed sequence
   high-water mark, agent and runtime provenance, branch, capture policy, and
   sync information when the export uses the cache.
2. `record`: one versioned history record per line, with sequence number and
   optional operator feedback. Records are ordered by capture sequence; native
   timestamps can be older than the preceding record when importing history.
3. `export_complete`: record count, high-water mark and SHA-256 of the exact
   UTF-8 `record` lines including their trailing newlines (excluding manifest
   and trailer). A failure after streaming begins emits `export_error` instead.

Require a valid `export_complete`, count and digest before ingesting. The export
pins both its initial sequence boundary and feedback snapshot: records arriving
while downloading appear in the next export. An intact export can still contain
unfinished episodes. For evaluation/RL, group by workspace and episode, filter
for appropriate terminal/delivery status, and deliberately handle gaps,
truncation, redaction, synthetic fixture data and imported/unknown metadata.
Do not treat a tool result or an agent's own claim as an operator reward.

`GET /v1/projects/{id}/audit-history` returns up to 20 cached events in reverse
cache insertion order. Use `before` with the returned `next_before` cursor to
read the next page. Optional `workspace_id` and `kind` filters select a worker
and event type. Each entry includes the worker ID and the versioned record.
The cache insertion cursor is independent of the worker's sequence and the
event's source timestamp.

An operator can annotate an existing run with
`POST /v1/workspaces/{id}/history/purpose` and
`{"episode_id":"…","purpose":"test"}`. Use `"normal"` to correct a classification.
Project/agent credentials cannot assign these labels. The platform stores only
the workspace ID, episode ID, purpose and annotation timestamp; exports pin the
purpose annotations at the start of the download. The manifest identifies
`purpose_source: "operator_annotation"`. Guest-supplied purpose claims are ignored.
