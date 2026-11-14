# Your first session with Agent Workbench

**For the conversation-first work flow, follow [the end-to-end work tutorial](work-conversations.md).** It covers instructions, recorded tool activity, saved outputs, follow-up changes and acceptance in the current UI. The terminal guide below documents the original synthetic task API.

This guide assumes `make up` has finished and the services are running. Allow about ten minutes. Use a terminal opened in the `agent-workbench` project folder and keep it open throughout the walkthrough.

**For interactive project development, start with [the web UI guide](interactive-workspaces.md)** and open `http://127.0.0.1:8080/workspaces`. The terminal walkthrough below covers the original fixture/task API. Existing workspaces may retain that earlier execution profile.

**What you have today:** a working local system for creating workspaces, submitting tasks, reading results and keeping their state between restarts. The dashboard shows activity; the command-line client and MCP interface operate the system.

**The default model is simulated.** A successful task returns a result beginning `[SYNTHETIC FIXTURE]`, followed by a hash. It does not answer your question, edit files or contact a real model. This first session checks that the system works. Connecting approved inference is the next step toward useful AI work. Use synthetic data while isolation and network containment remain unverified.

## 1. Open the dashboard

Open **http://127.0.0.1:8080**. No login is required.

The runtime overview is read-only. Its Workspace launcher link opens project controls; Open T3 opens the interactive coding interface. The terminal below remains available for task operations.

| Section | What to look for |
| --- | --- |
| Workspaces | The workspace's ID, state, readiness, activity, limits and planned idle stop |
| Active tasks | Work that is queued or running; finished tasks disappear from this list |
| Usage & quota | Running workspace count and allocated CPU/memory against the configured quotas |
| Services | Process health and when it was checked |

The page refreshes every five seconds. A task may finish too quickly to appear under Active tasks. An empty active-task list after completion is normal.

`Unverified` isolation/network and `Unknown` measurements are expected in this development setup. Healthy services do not imply verified security. Vault can be healthy while still sealed or uninitialized: its listener is running, but real credentials have not been provisioned.

## 2. Create your first workspace

A workspace is the persistent home for an agent's files, task records and results. Creating one initially leaves it stopped; submitting a task starts it automatically.

Paste this block into the terminal:

```sh
WORKSPACE_ID=$(python3 scripts/client.py create --key getting-started-workspace \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
printf 'Your workspace: %s\n' "$WORKSPACE_ID"
python3 scripts/client.py get "$WORKSPACE_ID"
```

The first command creates the workspace and saves its returned ID in a shell variable, so you do not have to copy a long identifier. `get` prints its details as JSON. Expect `state: STOPPED` for a newly created workspace.

The fixed `--key` makes this walkthrough safe to repeat: it returns the same workspace rather than creating another one. If you have already done the walkthrough, its current state may differ.

Refresh the dashboard if needed. Your workspace should appear in Workspaces.

## 3. Submit a task

```sh
TASK_ID=$(python3 scripts/client.py submit "$WORKSPACE_ID" \
  'Getting started: check that this workspace can accept a task.' \
  --key getting-started-task-v1 \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
printf 'Your task: %s\n' "$TASK_ID"
```

The system starts the workspace, waits for its task service, durably accepts the instruction and returns a task ID. The task continues independently of your terminal connection or dashboard tab.

On the dashboard, the workspace should become `READY` or briefly `BUSY`. Creating a workspace does not itself start an agent run; submitting this task does.

The client reads its local API credential automatically. You do not need to copy a token into these commands.

## 4. Check status and read the result

```sh
python3 scripts/client.py status "$WORKSPACE_ID" "$TASK_ID"
```

Look at `state` in the JSON response:

| Task state | Meaning |
| --- | --- |
| `queued` | Accepted and waiting to run |
| `running` | Executing |
| `succeeded` | Finished; read its result |
| `failed` | Finished unsuccessfully; inspect the safe `error_code` |
| `cancel_requested` | Cancellation requested; termination is not yet confirmed |
| `cancelled` | Execution has stopped following cancellation |

If it is queued or running, wait a moment and run the status command again. Once it succeeds:

```sh
python3 scripts/client.py result "$WORKSPACE_ID" "$TASK_ID"
```

The `content` field should begin `[SYNTHETIC FIXTURE] messages_sha256=...`. That is the expected demonstration response. It proves the request passed through the task service and inference gateway; it does not prove that a real model completed the instruction.

Results and instructions are deliberately absent from the dashboard. Read them through this client or an authorized MCP caller.

## 5. Prove that the result survives a restart

```sh
python3 scripts/client.py stop "$WORKSPACE_ID" --key getting-started-stop
python3 scripts/client.py get "$WORKSPACE_ID"
```

Expect `STOPPED`. Then start the same workspace and read the same task:

```sh
python3 scripts/client.py start "$WORKSPACE_ID" --key getting-started-restart
python3 scripts/client.py result "$WORKSPACE_ID" "$TASK_ID"
```

The result should be unchanged. It was retrieved from persistent workspace storage, rather than running the instruction again. Reading a task or result while its workspace is stopped also wakes that workspace automatically.

This preserves files and recorded task state. It does not resume a process that was interrupted by a crash; interrupted tasks are marked failed with `INTERRUPTED`, rather than replayed automatically.

## 6. Finish your session

Stop your workspace when you are done:

```sh
python3 scripts/client.py stop "$WORKSPACE_ID" --key getting-started-finish
python3 scripts/client.py list
python3 scripts/client.py usage
```

Alternatively, an idle workspace stops automatically after 60 minutes by default once all runs and background processes have finished. Dashboard polling does not keep it alive.

To stop the platform too, first stop any other workspaces you started, then run:

```sh
make down
```

The platform and dynamically managed workspace containers have separate lifecycles: `make down` alone does not stop workspace containers. It preserves the named storage volumes. Use `make up` to start the platform again.

Avoid `docker compose down --volumes` when you want to retain platform metadata and Vault storage.

## Everyday commands

Run these from the project folder. Keep the shell variables from the walkthrough, or replace them with IDs returned by `list` and task submission.

```sh
python3 scripts/client.py list
python3 scripts/client.py get "$WORKSPACE_ID"
python3 scripts/client.py start "$WORKSPACE_ID"
python3 scripts/client.py submit "$WORKSPACE_ID" 'A different synthetic test.' --key another-task-v1
python3 scripts/client.py status "$WORKSPACE_ID" TASK_ID_FROM_SUBMISSION
python3 scripts/client.py result "$WORKSPACE_ID" TASK_ID_FROM_SUBMISSION
python3 scripts/client.py cancel "$WORKSPACE_ID" TASK_ID_FROM_SUBMISSION
python3 scripts/client.py stop "$WORKSPACE_ID"
python3 scripts/client.py usage
```

`TASK_ID_FROM_SUBMISSION` is a placeholder: use the new task's returned `id`, not the literal text. Cancellation affects active work; a task that already finished keeps its terminal outcome.

Reuse a task key only to retry the **same** instruction. Reusing a key with different instructions produces `IDEMPOTENCY_CONFLICT`; choose a new key for new work. Omitting `--key` generates a new key, so repeating a submission without one creates another task.

If you open a new terminal, `WORKSPACE_ID` and `TASK_ID` will no longer be set. Run step 2 again to recover the tutorial workspace, and repeat the unchanged step 3 to recover its task ID. The data lives in the workspace, not in those shell variables.

## Optional: connect an external agent through MCP

After the terminal walkthrough works, an MCP-capable client can perform the same operations.

| Setting | Value |
| --- | --- |
| Transport | Streamable HTTP |
| Endpoint | `http://127.0.0.1:8082/mcp` |
| Authentication | `Authorization: Bearer <MCP credential>` |
| Credential source | The local file `.local/secrets/mcp_token` |

Enter that credential in your client's private header/secret configuration. It is distinct from the management API credential; do not use the token as part of the URL or paste it into a chat. Exact setup depends on the client. The current deployment represents one local operator in project `default`.

The server provides these ten tools:

| Tools | Purpose |
| --- | --- |
| `workspace_list`, `workspace_get` | Find and inspect workspaces |
| `workspace_create`, `workspace_start`, `workspace_stop` | Manage workspace lifecycle |
| `task_submit`, `task_status`, `task_result`, `task_cancel` | Submit and follow work |
| `usage_current` | Check current quota allocation |

For example, ask the connected agent:

> Use the Agent Workbench tools to create a workspace with idempotency key `mcp-demo-workspace`. Submit the instruction “Synthetic MCP onboarding test” with key `mcp-demo-task-v1`. Check its status, show the result when it finishes, then stop the workspace. Use the returned workspace and task IDs.

This still uses simulated inference. MCP gives an external agent control of the workbench; it does not itself connect a real workspace model or establish stock T3 attach compatibility.

For a client on another machine, use an authenticated SSH tunnel to the host's loopback endpoint. Direct remote access and client-specific setup are separate configuration work.

## If something does not work

| Symptom | What to do |
| --- | --- |
| Dashboard is empty | Run step 2. Workspaces are created on demand, not when the platform starts. |
| No task appears in Active tasks | Check its status in the terminal; it may already have finished. |
| You get a hash instead of a useful answer | Expected in fixture mode. A real inference provider has not been connected. |
| Terminal says the control plane is unavailable | Run `docker compose ps`; start the platform with `make up` if needed. Default API port is 8090. |
| `UNAUTHORIZED` | Use the matching deployment's local credentials. The CLI reads `.local/secrets/control_token`; MCP uses `.local/secrets/mcp_token`. |
| `IDEMPOTENCY_CONFLICT` | Retry the original instruction, or use a new task key for changed instructions. |
| `WORKSPACE_BUSY` when stopping | Let active work finish, or cancel its task and check that cancellation completes. Unknown/background activity also prevents stopping. |
| `QUOTA_EXCEEDED` | Run `usage` and stop an unused workspace you own. New interactive workspaces use one CPU and 1 GiB each; earlier task-only containers retain their original limits. |
| `RUNTIME_UNAVAILABLE` or a workspace stays `ERROR` | Check the runtime-controller logs and confirm the workspace image was built by `make up`. |
| `AGENT_PROTOCOL_UNAVAILABLE` from the connection API | Use Open T3 in the web workspace launcher. External desktop pairing is not configured by that endpoint. |
| Vault is sealed/uninitialized | Expected before real certificate integration; it is not needed for fixture tasks. |

Useful diagnostics:

```sh
docker compose ps
docker compose logs --tail=50 controlplane runtime-controller inference-gateway
```

For a repeatable synthetic functional check, run `make smoke`. It exercises task deduplication, inference, stop/start and result persistence, and leaves its test workspace stopped. It does not validate VM isolation or full egress containment.

These commands assume the default ports from `.env.example`. If you changed `CONTROL_PORT`, use `scripts/client.py --url http://127.0.0.1:YOUR_PORT ...` or set `CONTROLPLANE_URL` in your terminal. Use the configured dashboard/MCP ports for their URLs too.

## What comes after the walkthrough?

For Codex and Claude Code using subscription logins stored in Vault, follow [native subscription agents](subscription-agents.md). That replaces synthetic task execution with the native CLI inside newly created workspaces, with an explicit `codex` or `claude` provider available through MCP.

The next useful integration is an approved real inference provider, so tasks can use the implemented file, command and broker tools. Follow [gateway configuration](../config/README.md) for the policy and credential requirements; changing only `GATEWAY_MODE` is insufficient.

Real certificate-backed data access additionally needs initialized/unsealed Vault, a scoped broker credential and approved case/operation policies. Kata isolation, full host-level egress enforcement and stock T3 attach remain separate verification/integration gates. See [security boundaries](security.md) and the [service contracts](contracts.md) for the current implementation scope.

### Workspace connection profiles

Open **Workspaces → Settings → Connection profile**:

- **Blocked**: platform integrations only.
- **GitHub only**: HTTPS Git through `github.com:443`.
- **Custom allowlist**: edit one exact public hostname/IP and TCP port per line,
  for example `github.com:443`, `registry.npmjs.org:443`, `pypi.org:443` and
  `files.pythonhosted.org:443`. Include every host used by downloads/redirects.
  HTTP/HTTPS-aware tools use the injected proxy. Private/LAN targets, wildcard
  domains, direct guest DNS and arbitrary direct connections remain excluded.
- **Fully open**: direct networking and external DNS for all tools. This includes
  Internet, LAN and reachable host services. Use it only where that access is intended.

Connection policies are saved per workspace, independently of the bundled file/Git tools.
Stop before switching into or out of Fully open. Older proxy containers also need
one stop/start for custom rules. Finish active work and close terminals first;
files, conversation history, project and private home remain intact. No network
profile exposes Vault or provider subscription credentials.

Idle shutdown defaults to 60 minutes; select 15 minutes to 24 hours, or **Never
automatically stop**. Opening/starting resets the timer; dashboard polling does
not. Active runs and tracked background jobs prevent automatic shutdown.

### Global connection profiles

Choose **Connection profiles** in the sidebar, or visit
`http://localhost:8080/app/connection-profiles`. Edit the built-in profiles or
create named profiles with their own description, mode and approved destinations.
These settings are shared across all projects; only the local operator can edit
them. Project-scoped agent credentials cannot change global profiles.

Open **Edit profile**, choose **Delete profile**, then confirm to remove it.
If a workspace or Hermes setup uses the profile, choose another profile in those
settings first. Deleted built-in profiles stay deleted after a restart.

Choose a global profile when creating a workspace, or in **Workspace Settings →
Global connection profile**. Workspaces retain an applied policy/version. Global
edits appear as **update pending** until an explicit start/open or saving settings
applies the current version. Background polling and active tasks keep their
existing policy. A real stop/start is required when direct networking changes,
or when an older proxy needs updating. Files and conversation history are retained.

Choose **Workspace-specific settings** to detach from global updates and edit
that workspace's own allowlist. Existing workspaces remain detached until you
explicitly select a global profile; their access is not changed during migration.
Concurrent global edits use version checks and return a conflict rather than
silently overwrite a newer change.
