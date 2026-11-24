# Use Harbormaster

Open the dashboard URL your operator gave you. On a local Compose installation, use [the local dashboard](http://127.0.0.1:8080/app/) and sign in as `operator` with the password from `.local/secrets/dashboard_local_password`. On a hosted installation, sign in with an approved account and accept your project invitation if prompted.

## Open or create a project

Choose an existing project from **Projects**. If you can create projects, select **New project**, enter a name, and save it. When the project list is empty, **Add sample project** gives you planned tasks to explore without running an agent.

| Project page | Use it to |
| --- | --- |
| **Overview** | Create, assign, prioritize, and track tasks. Import issues from a connected repository. |
| **Workspace** | Send work to an agent, inspect files, review a preview, and approve or close a task. |
| **Workers** | Start and stop workers, open T3 Code, and inspect logs or message history. |
| **Settings** | Manage project settings, membership when available, and Hermes setup. |

## Invite a project member

1. If you can manage access, open **Settings**, then **General**. Find **Project members**.
2. Enter the person's sign-in email and choose **Member** or **Administrator**. Select **Invite member**.
3. Ask them to sign in with that address and select **Accept invitation** in **Projects**. The invitation appears in the app.

To remove access, use the revoke action on the member's row. Hosted sign-in must already allow the invited email.

## Plan a task

1. In **Overview**, select **Create task**. Enter the title and description, then choose a work profile.
2. Open the saved task to set its status, assignee, priority, or dependencies as needed.
3. To dispatch a planned task, select **Start work** when the task has an eligible agent and worker. To give an agent an instruction directly, start a thread in **Workspace**.

Changing a task's status or archiving it does not start or cancel an agent. For repository work, use **Import** in **Overview** and select the issues you want. Imports create local tasks. Use the repository actions in a task's details for explicit sync or refresh. See [repository accounts and issue sync](repository-connections.md).

## Start an agent thread

Before running real work, make sure the deployment has an approved agent and model. If personal subscriptions are enabled, add your account under **Connections**, then open **Settings** and **Subscriptions** on the worker to load it. Loading the subscription authorizes work on that worker to use your account. See [subscription setup](subscription-agents.md).

1. Open **Workspace** and select **New thread**.
2. Choose a model. The work profile defaults to **Web Development**. Select **Automatic worker** and confirm its default model to let Harbormaster provision a worker. Choose an existing worker if the work needs its files or its loaded subscription.
3. To keep specific output files, expand **Keep specific output files**. Enter one worker-relative path per line, up to 32. Ask the agent to write those files.
4. Select **Create thread**. When you’re ready to start work, enter a message in the thread and select **Send**. Switch to **Workspace** and select **Files** to inspect or download retained outputs.
5. Send another message for a follow-up, or select **Stop task** to request cancellation of an active run. Follow-up messages start new runs in the same thread and use the worker's existing files.

Closing the browser leaves accepted work running. A stopped worker's private conversation can require **Load conversation** before you can read it. The default local fixture returns a synthetic result in the task API. Use [the CLI fixture tutorial](getting-started.md) to verify that path, or [the agent conversation tutorial](work-conversations.md) to produce real files with a configured model.

| Work profile | Workspace |
| --- | --- |
| **Web Development** | Live web preview, markup, and agent output |
| **Desktop App Development** | Interactive Linux desktop for reviewing GUI apps |
| **Research / Data Science** | Datasets, interactive visualizations, and reports |

## Review a result

1. Open the task's thread and select **Workspace**.
2. Read the agent reply in **Conversation**. Open **Output** to see generated charts and images. Open **Files** for retained reports and code changes. In the live web preview, select **Markup** to mark a change.
3. Select **Approve this revision** to accept the output. To revise the result, send your feedback in the composer.

See [reviewing tasks in Workspace](reviews.md) for annotated images and agent review.

## Inspect an application

1. In the thread, switch to **Workspace**, then choose **Preview** and **Web**. If the worker already runs a detectable HTTP app, select **Open detected app** when prompted.
2. To have the agent start the app, select **Ask agent to run app** and send the prepared message. To start the app yourself, open **Preview options**, then **Preview settings**. Set the worker-relative directory, start command, and port. Select **Start preview**.
3. If startup fails, open **Application logs** from **Preview options**. Check the directory, command, port, and first error. In preview settings, select **Stop app** to end a managed application. Use **Disconnect preview** to remove the forwarding for an app started independently.

For a GUI application, choose **Desktop App Development** when you start the thread. Select **Start worker desktop** if a desktop session is not already running. The worker image must support desktop sessions. See [application previews, desktop, and charts](work-conversations.md#run-and-preview-a-web-application).

## Manage a worker

1. Open **Workers** and select **Create worker**. Choose **T3 with Codex** for T3 sessions or **Codex task worker** for task execution. Configure Hermes from **Settings**, on the **Hermes setup** tab.
2. Select **Open worker** for the worker you want to manage. If the worker is stopped, select **Check & start**. Resolve any reported blocker, then confirm **Start worker**.
3. Select **Open T3** for an interactive browser session. On a ready Codex worker without a console, open **Settings** and select **Enable T3** if available. To pair the desktop app, follow [local T3 pairing](interactive-workspaces.md#connect-t3-code-desktop).
4. To release compute, finish or cancel active work, close terminals, and stop managed applications. Select **Stop worker**. The worker keeps its files and history. The default idle timeout is 60 minutes, and active work delays automatic stop.

Project admins can select **Terminal** to connect to a project worker’s shell. Leaving the tab closes the shell. Use **Environment** to manage software tools and agent skills.

Use **Workers**, then **Project actions**, to open **Activity logs**, **Message history**, or **Copy a file**. For one worker, select **Open worker**, then **Activity**. Activity logs show progress metadata. Message history can contain private conversation text and tool payloads. Review any JSONL export before sharing it. See [activity logs](workspace-logging.md) and [history and exports](workspace-history.md).

## Change a worker's network access

1. In **Workers**, select **Open worker**, then **Settings**.
2. Select a global profile or use the worker's own settings. **Blocked** keeps platform integrations available. **GitHub only** allows HTTPS to `github.com:443`. **Custom allowlist** takes exact public host and port pairs, one per line. **Fully open** allows direct networking on Docker, including reachable LAN and host services.
3. Save the settings. Stop the worker first when changing into or out of **Fully open**. For a linked global profile, an explicit start or open, or a settings save, applies a pending version. Some running workers require a stop and start before the change can take effect.

Use **Platform**, then **Network profiles**, to manage reusable profiles if you have local operator access. Check the [runtime network boundaries](security.md#workspace-connection-profiles) before granting wider access.

## Connect tools and assistants

- To manage a project with an external MCP client, ask the local operator for [a project credential](project-workbench.md#connect-an-external-agent).
- To work with Hermes in the project UI, open **Project assistant** in **Workspace**. Use [Hermes setup](../deploy/hermes/README.md) for runtime requirements and supported Matrix deployments.
- To run scheduled tasks, use the [schedule API or MCP tools](contracts.md#schedule-management). Use the matching project credential and worker configuration.
