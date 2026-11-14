# Test a work conversation end to end

Use a running Workbench deployment with native Codex execution configured. The
synthetic fixture cannot create the files in this exercise. Allow a few minutes
for each run. The conversation shows recorded messages and tool activity as they
become available; it does not stream every generated token.

1. Open `http://127.0.0.1:8080/app/`, choose a project, then **Work → New
   conversation**. Select **Software** and an idle worker, or leave **Create a
   private Codex worker** selected.
2. Enter this instruction:

   ```text
   Create conversation-demo/add.py with an add(a, b) function. Add four unit
   tests in conversation-demo/test_add.py. Run the tests. Write a short report
   to conversation-demo/report.md describing the implementation, exact test
   command and observed results. Use only Python's standard library.
   ```

3. Expand **Keep specific output files** and enter one path per line:

   ```text
   conversation-demo/add.py
   conversation-demo/test_add.py
   conversation-demo/report.md
   ```

   Click **Start work**. The saved conversation opens and its URL includes
   `?work=...`; bookmark it if desired.
4. Stay in the conversation. Expand a tool call or result to inspect the agent's
   commands and output. The status changes from preparing/running to awaiting
   review. If the run fails, its error appears inline and **Technical details** contains
   the attempt/task references. A failed run cannot be accepted; use **Retry
   task** or send corrected instructions.
5. Open **Files**, then click **Inspect** on the report and tests. Verify that the
   report describes the command and the tool output confirms four passing tests.
   **Download** retrieves the retained file. A reply alone does not prove that
   files or tests exist; inspect the evidence.
6. Click **Request changes**, then send:

   ```text
   Extend add(a, b) to reject non-numeric arguments with TypeError. Add tests
   for both invalid argument positions. Rerun all tests and update the report
   with the new test count and actual results.
   ```

   The agent continues in the same conversation and uses the existing files.
   **Files** shows the current result. **Show earlier conversation** opens
   previous messages. During execution you can draft a message or click
   **Stop task**; sending waits until the agent finishes.
7. After checking the revised result, click **Accept result**. The work item
   becomes **Accepted**. Reload the page: the conversation, run history, outputs
   and accepted state should remain available. A later message starts another
   run in the same work item, preserving the earlier acceptance.
8. To check workspace persistence, let the worker become idle, stop it from
   **Workers**, and return to the bookmarked conversation. Click **Load
   conversation** to start the worker and read its private record. No task should
   rerun merely because the conversation is opened.

Follow-ups are fresh native runs with references to the prior instructions and
workspace files. They do not resume a native session or inject into a running
task. File contents and conversation bodies stay in the worker's private storage;
the control plane stores execution references and review state.

## Run and preview a web application

In a work conversation, select **Web → Run application** in the visual pane.
Enter the application's workspace directory and port (4173 by default). For a
static site, keep `python3 -m http.server "$PORT" --bind 127.0.0.1`; for a framework,
enter its start command and configure it to listen on the selected port. For the
Agent-workpence landing page the directory is `agent-workpence`.

Click **Start preview**; the preview appears in the pane when ready. HTTP
requests are forwarded to the application inside the workspace through an
isolated authenticated preview origin. The command, its output and files remain
in that workspace. No arbitrary host address can be chosen as a forwarding target.
**Application logs** shows startup errors; **Stop app** ends the process and
revokes access. Previews expire after one hour and keep their worker active while
running. A worker restart stops the application; start a new preview afterward.

This supports HTTP pages, assets, API requests and WebSocket-based live reload.
Use the refresh control for applications without automatic reload.

### Conversation and visual workspace

Open a work item to use the split workspace. The latest activity is shown by default;
**Show earlier conversation** restores previous messages, and **Files** opens
the current retained outputs and technical details. Tool actions are collapsed into groups. Drag the divider (or
focus it and use the arrow keys) to resize the conversation and visual pane.
On narrow screens, switch between **Conversation** and **Preview**.

The preview pane has two modes; charts appear in the conversation:

- **Web** runs an application and embeds its preview. Use the toolbar to reload,
  switch to a phone viewport, open a separate tab, or select an element. Selection
  adds a description to the message draft; explain the desired change and send it.
  Application commands, ports, logs, and stop controls are in **Preview settings
  and logs**. After changing a command, stop the existing app before reusing its port.
- **Desktop** starts a private X11 desktop in the workspace, viewed through noVNC.
  Mouse and keyboard input work inside the pane. Native agent runs started while
  the desktop is active receive `DISPLAY=:99`, so they can launch GUI applications.
  The runtime image must include `workbench-desktop` and its packaged dependencies.
- **Visualize data** in Conversation options opens retained or pasted CSV/JSON
  data. Agent-supplied charts appear directly inside replies. The browser draws
  them; no Python plotting library or application server is needed.

Application previews stream HTTP and WebSocket traffic through authenticated
runtime hops. Apps are bound to registered workspace services and loopback ports;
no operator credentials reach them. Embedded previews use isolated localhost
origins and partitioned session cookies. Stop and expiry terminate application
process groups. Preview sessions expire after one hour; reload to reconnect.

Managed application services and their supervised process descendants can coexist
with new conversation turns and private instruction-file handoffs. They still
count as background activity for idle stopping. Other active tasks, recipe jobs,
and unrelated background processes continue to block conflicting work.


### Debug a preview application

Click the terminal icon (**Application logs**) in the preview toolbar. The large
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


### Automatic ports and optional preview

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

**Collapse preview** leaves the running app connected and gives the conversation
the full width. **Show preview** restores it. Charts stay in the conversation,
including while preview is collapsed. Desktop mode still requires starting the
workspace desktop and an installed desktop-enabled runtime image.

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

Web preview reconnects automatically after an agent run finishes, including static
servers without hot reload. It keeps its session while the agent works and when
the pane is collapsed. A stopped application removes the old frame. Files and
manual chart entry are in Conversation options; preview tools and logs are in
Preview options. Generated charts and images stay beside the agent's reply.
When all previous app sessions stop, preview collapses to return the space to the
conversation. A newly ready app opens the pane again. Manually hiding an active
preview keeps its connection alive.


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
link it in the reply: `[Explore results](outputs/experiments.chart.json)`. Raw CSV
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
