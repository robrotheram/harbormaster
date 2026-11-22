# Workspace activity logs

Open **Workers**, then **Project actions**, then **Activity logs** for the project feed. For one worker, select **Open worker**, then **Activity** to see its logs. Filter by workspace or source and pause/resume
the two-second refresh while reading. The per-workspace view identifies the
Matrix listener separately from the run/process counts. Workspace rows also show
**Matrix listener: Connected** beneath the job count. Select it to open the
workspace activity log. Additional or unidentified processes stay visible as
separate background-process counts; they are not assumed to be the listener.

Records include task queue/start/finish/cancellation, native Codex and Hermes tool
categories, agent lifecycle, T3 session state changes, and Matrix connection,
message receipt and response processing. Claude task workers report lifecycle;
use their native interface for detailed tool output. Matrix "Response prepared"
means generation completed, not a delivery receipt from the homeserver.

Logs contain structured progress metadata. They exclude prompts, command
arguments, file contents, tool output, model reasoning, raw stderr and credential
values. Tool names are reduced to fixed categories; unknown names become `other`.
Malformed and oversized native log lines are discarded. Workspace emission is
limited to 240 events per minute, so this is an activity view, not an audit trail.

Each guest retains up to 2,000 events on its private disk and publishes its latest
200 through the authenticated task-status interface. The control plane collects
these without waking stopped workspaces and mirrors up to 2,000 events per
workspace. The UI/API show the most recent 500 matching records. Mirrored logs
remain readable after stop/restart and control-plane restart. A short task can
finish between polls: only its available terminal state may be captured on older
guests. Detailed events start when the updated guest image is installed; the
feature does not recover prior raw terminal history.

`GET /v1/workspaces/{id}/logs` returns that workspace's records and current named
services. `GET /v1/workspace-logs?project_id=…` returns the project feed. Existing
project authorization applies to both routes. Guest event fields are allowlisted
again before they enter platform metadata; workspace/project ownership is assigned
by the control plane rather than accepted from an event payload.

New Docker workspace builds and Firecracker guest images include the event
recorder. Existing VMs need a clean restart into the updated image. Installation
must preserve their private disks and restore any previously connected Matrix
listener; active workspace runs must finish before upgrading.
