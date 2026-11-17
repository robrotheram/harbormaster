# Reviewing tasks

Open **Reviews** in a project. The queue shows readable work titles and selects the first pending task. **History** contains finished tasks. Search matches titles, work IDs and kinds.

Choose the format that fits the task:

- **UI preview** opens the current workspace application or desktop. Live content can change after the task finishes.
- **Code review** displays retained source/patch files and captured file edits, with line numbers and added/removed lines. Recorded edit requests are explicitly identified: they may have failed, and captured activity is not a complete repository diff. Earlier evidence can be loaded for longer tasks.
- **Visualisations** displays delivered image/chart references and retained CSV or JSON data using the existing interactive chart viewer.
- **Reports** renders the worker’s last recorded response and retained Markdown/text reports. If no response was captured in history, it uses the task result. Earlier responses remain available. Other document formats open through the original artifact link.

Review is the final stage of a task. Each task shows its current result, with no run selector or comparison between runs. The conversation and Files view also focus on the current result; earlier conversation messages are still available. Internal execution records remain intact for auditing and concurrency checks. Only pending work can be approved or sent back for changes. The server checks the expected attempt ID before recording a decision. A successful decision refreshes both Reviews and the issue board.

Select **Approve** to approve the work. Select **Close** when no further work is needed but you do not want to approve it. Close records a separate `closed` work state and review outcome, removes it from the pending queue, and places it in History and the board’s Done column. It does not start another attempt. To request changes, select **Request changes**, enter a comment in **What needs changing?**, and select **Send changes**. **Cancel** hides the comment box and keeps the draft while this task is open. The comment is saved as the next work request and starts a revision. Preview element selections open and populate the same comment box. The action bar stays below the scrollable result, and the task description is available beside its status.

Preview startup is separate from work revisions. **Start preview** reuses a saved application launch configuration; **Open detected app** connects to a running server. If no launch command is available, **Set up preview** asks for the directory, command, and port. Starting a preview uses the application service API and does not create an attempt, change the work title, or remove the review from the queue.

Each mounted live preview obtains a fresh single-use connection ticket, including after a readiness change, reload, or reopening the preview. Ordinary polling does not reload a working preview.

Artifact previews are bounded and fetched only through the authenticated local API. Markdown does not execute HTML. Missing or truncated evidence is identified in the viewer. Image/data references in recorded results use retained outputs; they cannot silently read files from a newer execution.

To review finished work again, move it from Done to Review on the issue board.
Moving it to Planned or In progress reopens the task for further work without
starting the agent. The previous approval or closure is retained in review
history; the current decision is cleared.

Request changes also supports **Capture screenshot** (choose a browser tab, window,
or screen) and **Attach screenshot** (PNG/JPEG). Add a note to each image, remove
unwanted captures, then send the changes with your overall comment. Up to four
screenshots are resized to a maximum dimension of 1920 pixels and limited to
1 MiB each. Draft images stay in memory for the current session; sending saves
immutable private workspace files referenced by the revision request. Browsers
without screen capture support can still attach existing screenshots.

**Agent review** starts a separate research work item in the same workspace,
using the task's agent selection. Add an optional focus, open its conversation,
or copy its returned findings into the change notes for editing before sending.
The review is instructed to inspect and report without making changes; this is
an agent instruction, not a read-only workspace sandbox. It does not decide the
original review. The source revision and retained outputs are included in its
request; workspace files remain live. The separate review remains available in
Work after leaving the page.
