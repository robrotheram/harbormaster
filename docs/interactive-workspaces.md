# Develop through the workspace web UI

Open **http://127.0.0.1:8080/workspaces**. This is the local workspace launcher. The Runtime overview remains an operational view; the launcher can create, start, stop and copy workspaces.

Codex subscription setup must already be complete; see [subscription agents](subscription-agents.md). Claude remains disabled until separately configured. `make up` builds a workspace image containing pinned T3 Code 0.1.3, Codex, Claude Code, local MCP servers, Git, Python, Node and ripgrep. No provider credentials or host source directories are included in that image.

## Create a project workspace

1. Enter a name, such as **Photo gallery**.
2. Choose the agent: T3 for browser/desktop development, or Codex for task execution.
3. Leave **Start from** set to **An empty project**, then select **Create workspace**.
4. Select **Open T3** on its row. Allow a new browser tab if prompted.

The new tab runs T3's actual bundled web client. Its native HTTP and protocol-2 WebSocket requests reach a T3 server **inside that workspace container**. Use the T3 interface to create threads, chat with Codex, inspect changes and open a terminal. Its project root is `/data/files`.

For a first real task, send:

> Build a static photo gallery with local placeholder images, responsive CSS and a structure check. Use workspace-files for a file operation and project-git to inspect status. Do not download dependencies or publish anything.

The agent, terminals and MCP subprocesses all run in that workspace. Files and T3's conversations, environment identity and execution history persist on its own volume. Closing the browser detaches the client; it does not cancel an agent run.

T3 and native Codex/Claude workers include the same two bundled MCP servers:

| MCP server | Tools |
| --- | --- |
| `workspace-files` | Confined read, write and directory listing, up to 64 KiB per file |
| `project-git` | Read-only Git status, diff summary and recent history |

These are fixed workspace tools, not separate credential authorities. Full-access coding agents can run programs in their own container; the network and credential boundaries are enforced outside it. Additional MCP software must be baked into an approved image, and remote MCP endpoints need their own allowlist and credential policy. No MCP receives a Docker socket, Vault token or provider subscription token.

Workspace names do not enable external integrations or grant network access. Tool-profile management and selection have been removed; T3 and native Codex/Claude workers use the bundled file/Git tools. Hermes retains its native toolsets. Existing private homes and workspace data are preserved. After upgrading the Docker controller, it refreshes existing containers on their next start from a stopped state and reuses their data volumes.

## Separate workspaces for branches and PR work

On the parent project's row, select **Branch workspace**. Enter a name and a new branch, such as `pr-42-gallery-layout`, then create it and open T3.

The parent must be idle: finish agent runs and close terminal sessions first. On first start, the controller freezes the parent briefly while copying only `/data/files` into the new workspace's separate volume. It then resumes the parent and creates the requested local branch in the new workspace. Instructions, task results, T3 conversations, provider sessions and grants are not copied. A failed or incomplete copy cannot start the target agent.

This copies the current local project, including uncommitted project files and its Git data. It is not a linked host Git worktree and does not fetch a remote PR branch, push a commit or create/merge a PR. Git-host authentication and approved Git network access remain separate integrations. T3's own worktree features create checkouts within one workspace; use **Branch workspace** when you want another container and volume.

The parent and child can run independently and work on different changes. New interactive workspaces allocate one CPU and 1 GiB of memory; the default platform quotas allow four running workspaces with 4 GiB allocated in total. Existing task-only containers keep their previous limits and execution profile.

## Connect T3 Code desktop

1. Open the [workspace launcher](http://127.0.0.1:8080/workspaces) on the Docker host.
2. On the workspace row, select **Connect desktop**. This starts it and creates a native, one-use T3 pairing link.
3. Select **Copy pairing URL**.
4. In T3 Code desktop, open **Settings → Environments → Add environment**, choose the direct URL connection when offered, and paste the complete pairing URL.

Use the native T3 desktop version compatible with the bundled server (currently 0.1.3, orchestration protocol 2). The `desktop-ws-….localhost:8080` address works on the same computer as Docker. It is different from the browser-only `ws-….localhost` address. Do not enter `inference-gateway:8080` as an environment URL; that address is internal to Docker.

The desktop sees the workspace's existing project, conversation history, agent and terminal sessions. No project copy is needed to switch between browser and desktop. Native T3 exchanges the link for a client credential and checks that client's scopes on HTTP and WebSocket calls. The gateway does not lend the desktop the supervisor's privileged browser credential. A link can be used once; create a new one if it expires. Revoke unwanted clients in that environment's native T3 Sharing & access settings.

Stopping or idle-stopping the workspace disconnects the desktop. Select **Open T3** or **Connect desktop** in the launcher to start the same workspace again; the saved desktop connection can then reconnect. The desktop connection does not itself keep an idle workspace running.

This local route does not configure T3 Connect, SSH launching, remote-host access or public exposure. See [T3 direct pairing](https://github.com/pingdotgg/t3code/blob/main/docs/user/remote-access.md#pair-over-a-lan-or-private-network).

## Share files between workspaces

Open **Share a file between workspaces** in the launcher. Select the source and destination workspace, enter the source file and a new destination path, then select **Copy file**. Paths are relative to each workspace's `/data/files`, for example `reports/findings.md` → `incoming/findings.md`.

Both workspaces must be running and idle; finish active turns and close terminals first. One regular file up to 1 MiB is copied per operation, including binary files. Parent directories are created. The destination is published as a complete new file and existing files are never replaced. Choose another destination name when one already exists. A lost response can mean the copy completed: check the destination before retrying.

The source remains unchanged. This is a one-time copy, not continuous synchronization or a shared writable directory. Only the explicitly selected project file crosses the boundary; T3 conversations, agent homes, subscription credentials and workspace grants stay separate. Git metadata and named credential/configuration paths are rejected. The receiving agent should treat content from another workspace as untrusted input.

Content passes through the trusted services in bounded memory; it is not written to PostgreSQL or platform logs. Use **Branch workspace** for a complete initial project copy instead of copying files individually.

## Browser access and stopping

Selecting Open T3 first starts the workspace, then issues a one-minute, one-use connection ticket. The browser exchanges it on that workspace's own `ws-….localhost` origin for a host-only, HttpOnly, SameSite session cookie. The ticket travels in a URL fragment and is removed from browser history on exchange. Each browser session is bound to one workspace and lasts up to twelve hours; reopen from the launcher when it expires. The actual native T3 bearer remains inside the workspace supervisor.

Only the existing loopback dashboard port is published. No per-workspace host port is exposed. Cross-origin launcher mutations, WebSocket connections and cross-workspace browser-cookie reuse are rejected. This is still the agreed **single local operator without an account login**: another local process that can reach the launcher can operate it. Remote/public access and multiple-user authorization are not implemented by this UI.

To stop a workspace, let the agent finish and close open T3 terminals, then select Stop in the launcher. Active, background and unknown activity prevents shutdown. T3's idle server, resource monitor and registered protocol helpers do not themselves keep a workspace busy. The controller uses T3's authoritative thread state together with process observation; browser disconnects never imply task completion.

The existing fifteen-minute idle stop still applies after work finishes. If a workspace stops while a browser is open, return to the launcher and select Open T3. The same files and conversations reappear.

Old task-only workspaces are retained. Creating a new workspace is the supported path to the interactive image; no existing volume is deleted or host repository silently imported.

## Current limits

- VM isolation and full host-level egress containment are still unverified. A healthy T3 session does not establish either boundary.
- Subscription access tokens are snapshots in Vault. Refresh the normal operator login and sync again when they expire; there is no API-key billing fallback.
- Private-repository import, package downloads, Git-host PR actions, preview-browser installation and banking integrations need explicit additional configuration. The initial image supports local development with its baked tools.
- Browser UI, desktop pairing and native WebSocket access use the bundled T3 version. Exposing this gateway to another machine remains separate work.

See [security boundaries](security.md) and [service contracts](contracts.md) for the remaining production gates.
