+++
title = "Application preview and output behaviour"
+++

Start with the [dashboard preview guide](../application-previews.md) for the user workflow and [service topology](../architecture/_index.md#service-topology) for the trust boundaries.

## Follow a preview request

1. The dashboard requests an application connection through the control plane.
2. The control plane authorizes the project and the worker. It issues a scoped preview connection.
3. Dashboard ingress serves the frame on an isolated preview origin. The runtime controller forwards the allowed traffic to the worker.
4. The worker forwards HTTP and WebSocket traffic to the selected in-worker application. The application does not receive management credentials.

| Area | Code |
| --- | --- |
| Preview controls, log viewer, and rendering | [Dashboard work feature](../../../../dashboard/src/features/work) |
| Preview session policy | [Preview service](../../../../internal/previewsvc) |
| Preview origin and proxy | [Dashboard ingress](../../../../internal/dashboard) |
| Application supervision and forwarding | [Workspace supervisor](../../../../internal/workspace) |

## Managed and detected applications

A managed start uses the supervisor. The guest command `workbench-preview start --directory <relative directory> --port <port> --command <command>` waits for readiness and returns a service ID. A stable `--key` can retry a start. Use `workbench-preview stop --id <service ID>` to stop it. Managed applications remain active after the agent invocation ends and count toward worker activity.

Discovery checks eligible listeners every three seconds while the work conversation is open. It checks up to 32 listeners and skips reserved management ports. The preview tracks the detected socket identity. A restarted app needs a new connection. Disconnecting a detected preview only revokes the forwarding. The worker supports up to four active preview services.

Previews expire after one hour. A worker restart stops its applications. The worker keeps the last 64 KB of managed application output in memory. That buffer tracks stdout and stderr and is cleared by a restart. Test expiry and reconnect behaviour when changing the proxy or the preview UI.

## Reply files and images

Use retained artifact bytes for historical revisions. The latest reply can also link an image in private worker storage up to 1 MiB. Render SVG as an image resource. Scope reads to the authorized workspace and reject external URLs. See [guest artifacts](../architecture/guest-execution.md#publish-and-read-artifacts).

## Interactive charts from workspace data

A new conversation or follow-up sent from the UI includes the versioned chart
contract in its private agent instructions. Small results can use a
`workbench-chart` fenced block in the reply:

````markdown
```workbench-chart
{"version":1,"title":"Experiment scores","type":"bar","x":"experiment","y":["score"],"data":[{"experiment":"Baseline","score":0.71},{"experiment":"Revision","score":0.84}]}
```
````

For larger results, write the same object to `outputs/experiments.chart.json` and
link to the file in the agent reply. Use the following Markdown link.

```markdown
[Explore results](outputs/experiments.chart.json)
```
Raw CSV or JSON arrays of row objects also work; the UI chooses initial axes. Linked files
are read from the latest attempt's private workspace, or from a retained artifact
for a historical attempt. Retain the data file if it must remain available after
later runs. Inline chart data remains part of the private conversation record.

Charts support bar, line, area, scatter and donut views. **Explore** reveals chart
type, axis and metric selection, comparisons, sorting and row filtering. Larger
axis charts offer zoom. **Data** shows the underlying values. Select a point or
inspect a table row, then **Ask agent about this** to prepare a question in the
composer; review and send it to start the next run. Export the filtered CSV,
original chart JSON, or current SVG from the chart menu.

Each chart accepts at most 256 KiB, 2,000 rows, 32 columns and six numeric metrics.
Use explicit null for missing values; they remain gaps rather than zeroes.
Scatter plots require a numeric horizontal axis. Donuts require one nonnegative
metric. Optional `description` records scope and caveats; `unit: "bytes"` formats
values as B/KiB/MiB/GiB. The format accepts data and declared settings only, not
HTML, scripts, remote data URLs or arbitrary chart-library configuration.

## Validate changes

Run `go test ./internal/previewsvc ./internal/dashboard ./internal/workspace` for session, proxy, and supervisor changes. Open [the dashboard browser suite](../../../../dashboard/tests/work.spec.ts) for embedded preview, expiry recovery, and reply rendering cases.
