# Use an interactive worker

Open the dashboard and select your project's **Workers** page. Configure [native inference](subscription-agents.md) before submitting real tasks in T3. The [user guide](user-guide.md#manage-a-worker) covers worker creation and start checks.

## Open T3 in the browser

1. Create a worker with **T3 with Codex**, or select **Open worker** on a ready Codex worker. In **Settings**, select **Enable T3** if available.
2. Open the worker and start it if needed. Select **Open T3**. If the browser blocks the new tab, allow popups for the dashboard and retry.
3. Use the T3 tab to create a thread, inspect changes, or open a terminal. The project root is `/data/files`.

T3 runs in the worker. The browser uses a workspace-specific origin and a one-use connection ticket. If the session expires, select **Open T3** again on the worker page. Closing the tab leaves active work running. Use **Stop worker** when you finish and the worker is idle. A stop preserves files, agent settings, and recorded history.

Bundled `workspace-files` provides confined file operations, and `project-git` reports Git status, diff summaries, and recent history. Runtime network access follows the worker's connection profile.

## Branch a worker

1. On a backend that supports branch seeding, start the parent and wait for it to become idle.
2. Open the worker. In **Settings**, select **Branch worker**. Enter a worker name and a new local Git branch.
3. Create the worker and open it to check the copied files. The copy uses separate storage and excludes the parent's conversations and agent settings. Publish the branch through your usual Git workflow if needed.

## Connect T3 Code desktop

1. On the local operator dashboard, start a T3-enabled worker. Open the worker. In **Settings**, select **Connect T3 desktop**.
2. Select **Open T3 desktop** to hand the URL to the installed app. To enter it manually, select **Copy pairing URL**. In T3 Code, open **Settings**, then **Environments**, then **Add environment**. Paste the full URL.
3. Confirm the worker in T3 Code. If the link expired or was used, request another from the worker’s **Settings**.

The one-use URL grants access to the same files and conversations as the browser session. Keep it private. To revoke a client, use the environment's native T3 access settings. Hosted dashboards disable native desktop pairing. Use the browser console for hosted access.

## Share files between workers

1. In the same project, start both workers and wait for them to become idle.
2. In **Workers**, open **Project actions** and select **Copy a file**.
3. Select the source and destination workers. Enter a source path and a new destination path relative to each worker's project root.
4. Select **Copy file**. If the response is lost, check the destination before retrying.

A copy accepts one regular file up to 1 MiB and creates parent directories as needed. If the destination path already exists, choose a different path. Git metadata and credential paths are excluded. Use **Branch worker** to copy a project baseline on a supported backend.
