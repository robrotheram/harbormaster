# Test a work conversation end to end

Use a running Workbench deployment with native Codex execution configured. The
synthetic fixture cannot create the files in this exercise. Allow a few minutes
for each run. The conversation shows recorded messages and tool activity as they
become available; it does not stream every generated token.

1. Open the dashboard and choose a project. Select **Workspace**, then **New thread**. Select **Web Development** and an idle worker with native inference configured. Use **Automatic worker** if the deployment supports provisioning a worker for your configured agent.
2. Choose a model. Expand **Keep specific output files** and enter one path per line.

   ```text
   conversation-demo/add.py
   conversation-demo/test_add.py
   conversation-demo/report.md
   ```

3. Select **Create thread**. Bookmark the conversation URL, which includes `?work=...`. Enter the following message in the thread and select **Send**.

   ```text
   Create conversation-demo/add.py with an add(a, b) function. Add four unit
   tests in conversation-demo/test_add.py. Run the tests. Write a short report
   to conversation-demo/report.md describing the implementation, exact test
   command and observed results. Use only Python's standard library.
   ```

4. Stay in the conversation. Expand a tool call or result to inspect the agent's
   commands and output. The status changes from preparing/running to awaiting
   review. If the run fails, its error appears inline and **Technical details** contains
   the attempt/task references. A failed run cannot be accepted; use **Retry
   task** or send corrected instructions.
5. Switch the thread to **Workspace** and open **Files**. Inspect the report and tests. Verify the command in the report and check the recorded tool output for four passing tests. Use **Download** to retrieve a retained file.
6. Enter the following feedback in the Workspace composer and select **Send**.

   ```text
   Extend add(a, b) to reject non-numeric arguments with TypeError. Add tests
   for both invalid argument positions. Rerun all tests and update the report
   with the new test count and actual results.
   ```

   The agent continues in the same conversation and uses the existing files.
   **Files** shows the current result. **Show earlier conversation** opens
   previous messages. During execution you can draft a message or click
   **Stop task**; sending waits until the agent finishes.
7. After checking the revised result, open the task’s **Workspace** and select **Approve this revision**. The work item becomes **Accepted**. Reload the page and open the conversation again. Its history, outputs, and accepted state should still be available.
8. Stop the idle worker from **Workers**. Reopen the bookmarked conversation.
   The latest synced messages and recorded activity appear without starting the worker.
9. To continue on another worker, select it under **Worker for next message**.
   Send a follow-up. Open **Show earlier conversation** to check the earlier
   replies. The new run uses the selected worker and the same thread.

A follow-up starts a fresh run. A move gives the new worker a bounded copy of
the cached requests and replies, using the recorder's redaction. It needs the
latest request in the central cache. Project files, services, and native
sessions stay on the original worker. Each attempt keeps its worker and artifact
references. See [agent history and evaluation export](workspace-history.md) for
cache coverage.

## Run and preview a web application

In the thread, switch to **Workspace**, then choose **Preview** and **Web**. To start the app yourself, open **Preview options**, then **Preview settings**. Enter the app's directory relative to the worker's project root. Set the port, which defaults to 4173. For a static site, use `python3 -m http.server "$PORT" --bind 127.0.0.1`. For a framework, use its start command and configure it to listen on the selected port.

Select **Start preview**. The pane opens the preview when the app is ready. HTTP
requests are forwarded to the application inside the workspace through an
isolated authenticated preview origin. The command, its output and files remain
in that workspace. No arbitrary host address can be chosen as a forwarding target.
**Application logs** shows startup errors; **Stop app** ends the process and
revokes access. Previews expire after one hour and keep their worker active while
running. A worker restart stops the application; start a new preview afterward.

This supports HTTP pages, assets, API requests and WebSocket-based live reload.
Use the refresh control for applications without automatic reload.

### Conversation and visual workspace

Switch between **Conversation** and **Workspace** at the top of the thread. **Conversation** shows messages and recorded tool activity. Use **Show earlier conversation** to load earlier messages. The message composer stays open in either view.

In **Workspace**, **Preview** shows the live app when available. The **Output** view appears when charts or images are available. Use **Files** to inspect or download files and **Settings** to manage the task.

- With **Web Development**, select **Markup** in **Web** to annotate the running app. Open **Preview options** to change the viewport, open a separate tab, or select an element. Element selection adds text to your message draft. Describe the change before sending.
- With **Desktop App Development**, open **Desktop** to use the worker's Wayland session. If the session is stopped, select **Start worker desktop** or run `workbench-preview desktop` in the worker. Use `workbench-desktop run <app> [args...]` to launch a Wayland or X11 app from a worker shell. Open app windows survive a browser reload while the session is active.
- While you use **Workspace**, new replies continue to arrive. Select **View message** to return to **Conversation** without clearing the message draft.

Managed application services count as background activity for idle shutdown. You can continue the conversation while a managed app runs.

### Debug a preview application

Open **Preview options**, then **Application logs**. The large
viewer includes stopped sessions, their commands and directories, and stdout/stderr.
Use **Filter log lines** to find an error, **Pause** to freeze incoming updates,
and **Follow latest** to jump to and follow new output. **Wrap** toggles long-line
wrapping. **Copy** copies visible (filtered) lines; **Download** saves the entire
current log buffer. **Ask agent to debug** prepares an editable conversation draft
with the application configuration and up to 6,000 characters of visible logs.
Review that draft and send it to start an investigation.

The workspace keeps the latest 64 KB per application in memory. This viewer does
not include browser JavaScript console errors or recover logs from a restarted
workspace.


### Open a detected HTTP service

The work conversation detects HTTP listeners bound to workspace loopback or all
interfaces (IPv4 and IPv6). An eligible server on port 3000, for example, appears
in Web preview automatically; a localhost URL printed in an agent message is not
a URL on your own machine. Keep the server running in the workspace. Discovery
runs every three seconds while the work conversation is open, checks at most 32
listeners, and never probes reserved management ports. Up to four preview services
can be active. HTTPS-only and non-HTTP ports are not automatically forwarded.

Each forward is bound to the detected socket identity, rather than just its port.
A restarted server receives a new preview connection. **Disconnect preview**
revokes forwarding without killing an application the agent started independently.
Its process output stays in the conversation’s tool activity; only applications
started through the preview launcher have captured application logs.

Switching back to **Conversation** keeps the preview connected. Use **Disconnect preview** when you want to revoke forwarding.

### Agent-owned app startup

Ask the agent to run the app from the conversation. Native agents are instructed
to use the bundled `workbench-preview start --directory <relative directory>
--port <port> --command <command>` command. It creates a supervised application
inside the worker, waits for readiness, and returns the service identity. Unlike
an ordinary background command in an agent terminal, this process survives the
native run's process-group cleanup. `workbench-preview status` checks the running
services; `workbench-preview stop --id <service ID>` stops one. A stable `--key`
can be reused to retry a start. The helper uses the workspace's generation grant
and never passes it into the application process.

The preview empty-state action prepares an agent request instead of asking the
user to enter a server command. Once the agent starts the service, Web preview
connects through the workspace proxy. The container's localhost URL is not a URL
the user must open on their own machine.

Agent replies render Markdown, including generated SVG and raster images. A
retained image uses its immutable artifact. Images referenced by the latest reply
can also be read from that work item's private workspace (up to 1 MiB), without
requiring the user to predeclare an output path. Older unretained images are not
silently replaced with newer files. SVGs render as image resources, not executable
HTML. External images and paths outside the workspace are not loaded.

The web preview refreshes after an agent run finishes. It keeps the preview session while the agent works and clears the frame when the application stops. Use **Output** for charts and images and **Files** for file inspection.

### Interactive charts from workspace data

Ask the agent to collect measurements and show an interactive chart. Python can
still collect or analyse data inside the workspace; drawing and interaction happen
in your browser. For example:

```text
Measure the current disk usage. Show an interactive donut of used, available,
and reserved bytes, and a bar chart comparing top-level workspace directories.
State the measurement time and scope. Do not start an application server.
```

A new conversation or follow-up sent from the UI includes the versioned chart
contract in its private agent instructions. Small results can use a
`workbench-chart` fenced block in the reply:

````markdown
```workbench-chart
{"version":1,"title":"Experiment scores","type":"bar","x":"experiment","y":["score"],"data":[{"experiment":"Baseline","score":0.71},{"experiment":"Revision","score":0.84}]}
```
````

For larger results, write the same object to `outputs/experiments.chart.json` and
link to the file in the agent reply. Use `[Explore results](outputs/experiments.chart.json)` as the Markdown link. Raw CSV
or JSON arrays of row objects also work; the UI chooses initial axes. Linked files
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
