# Hermes workspace and Matrix connection

The supported runtime is Hermes **v0.23.0**, pinned to upstream commit
`2bd378ed1b8d6bf2192cb26e0a55ff3091e6bd19`. It runs inside the existing
Firecracker microVM, with no network card. The guest build uses upstream's frozen
`uv.lock`, including Matrix encryption dependencies. The host installation and a
real VM import check must pass before Hermes appears as available in the UI.

## Use the workspace agent

1. Open a project and select **Hermes setup** in the sidebar.
2. Choose **Workspace agent and Matrix bot** (or workspace only), enter an approved
   model, and select Blocked or an approved connection profile.
3. Select **Save Hermes configuration**, then **Start Hermes workspace**.
4. Open that workspace, select **Run task**, and submit an instruction. Hermes can
   use its terminal, file and memory tools within that workspace.

Both mode can be saved before Matrix details are available. Provider requests use
the existing Workbench Codex subscription gateway and its host-held native login.
The VM receives a generation-bound workspace grant, never the provider credential.
API-key and alternate-provider fallback are not configured. Account allowance and
provider terms remain properties of the connected account.

Workspace files, Hermes history and Matrix crypto state persist on the private
workspace disk. Stop/start preserves them. Saved model and Matrix changes apply
at the next workspace start. Connecting Matrix automatically restarts an idle
workspace when its saved Matrix settings have changed; busy workspaces are refused. Fully open networking is refused.

## Connect Matrix

Use a dedicated bot account and device for this project. It must belong only
to configured encrypted rooms, or have invitations to them for Workbench to accept.
Creating the account and sending its first invitations remain server/client actions;
Workbench does not need Matrix administrator credentials.

1. In **Hermes setup**, enter the HTTPS homeserver URL, bot user ID, exact room IDs
   (`!…`) and authorized sender IDs (`@…`), one ID per line.
2. Save the configuration. Workbench discovers the Matrix homeserver from an
   Element address and checks the resulting endpoint before any token is sent.
3. Enter the bot's access token in **Bot access token**, then select
   **Save bot credential**. Validation reports the specific prerequisite that is
   missing: bot identity, invitation, membership or required room encryption.
   Invite the bot from your own Matrix account if needed. Select **Accept
   invitations to the configured rooms** and retry to accept those invitations
   from Workbench. Only exact configured room IDs can be joined, and the server
   must show a pending invitation for the bot. Foreign room invitations are ignored.
4. Select **Connect Matrix**. Complete device verification in your Matrix client
   if required. The status changes to Connecting, Connected or Connection failed.
5. Select **Disconnect** before stopping a workspace with a running bot.

The token field is write-only. The root host controller stores it under
`/etc/agent-workbench/firecracker/matrix/` with restrictive permissions. Neither
project settings nor the guest disk receives the real token. Only local operator
authority can save credentials or enable messaging; project agent credentials
cannot connect a bot themselves.

The conditional vsock channel on port 18084 terminates at a host HTTP broker. It
binds one running VM generation to the configured homeserver, replaces the guest
grant with the bot token, rejects redirects and private network destinations,
allows encrypted sends only to exact project rooms, and filters sync events
before they reach Hermes. Foreign rooms, invitations, knocks and unauthorized
timeline senders are removed. Joins and invites, arbitrary event history, media
downloads and plaintext sends are denied. This adapter currently supports text
chat. Required encryption fails closed when its dependencies or crypto setup fail.

Upstream's room environment variable alone is insufficient: its Matrix adapter
exempts two-member DMs and can auto-join invitations. The host broker applies the
same exact-room boundary to those cases. The native callback also checks room and
sender before creating an agent session. Host broker unit tests cover these
boundaries; a real encrypted conversation still requires your Matrix account,
rooms and credential.

The API is `GET/POST /v1/projects/{id}/hermes`, with operator-only
`POST …/hermes/discover`, `POST …/hermes/matrix-credential` and
`POST …/hermes/matrix/start|stop`. Credential submission carries the displayed
settings revision and an explicit `accept_invitations` boolean. Discovery follows
no redirects, sends no credentials, and uses the same public-address network policy
as the gateways. The guest broker still forbids all joins; invitation acceptance
is a separate local-operator onboarding operation.
No credential value is returned by a read endpoint.

## Install or update the runtime

Follow [Firecracker installation](../../docs/firecracker.md). Staging builds the
Hermes engine into the guest image; installation checks its imports inside a real
VM. Stop workspaces cleanly and stop the host controller before replacing its
artifacts. Do not restart Vault as part of a runtime update.

The older `deploy/hermes/compose.yaml` is a separate, onboarding-only version
probe. It does not install the supported workbench runtime or connect Matrix.
The `project-config.example.yaml` file is a planning template, not a runtime
configuration file. Workbench MCP scheduling from Hermes is not wired into this
adapter.

Sources: [pinned upstream source](https://github.com/NousResearch/hermes-agent/tree/2bd378ed1b8d6bf2192cb26e0a55ff3091e6bd19),
[Matrix adapter](https://github.com/NousResearch/hermes-agent/blob/2bd378ed1b8d6bf2192cb26e0a55ff3091e6bd19/plugins/platforms/matrix/adapter.py).

## Adapter maintenance

The Firecracker guest's pinned Hermes identity and source checksum are defined
in `runtime.json`. `scripts/firecracker-stage.py` uses this manifest for the
Docker build arguments, installed guest identity and host admission record.
Task and Matrix entrypoints share `scripts/hermes_common.py` for their fixed
inference route, private no-fallback configuration and versioned event framing.
Rebuild and restage the guest when changing these files.

Run `python3 scripts/test-hermes-adapters.py` for portable routing, authorization,
delivery and failure tests with SDK import stubs and no network. The existing
`test-hermes-runner.py` and `test-hermes-matrix.py` can also run in the guest with
its real pinned SDK; portable tests do not replace that image acceptance check.

### Fixed project MCP channel

Firecracker Hermes guests use loopback port 18085, bridged over vsock to the
VM-specific host listener. Only `POST /mcp` is accepted, with the current workspace
generation grant. The host forwards only MCP initialization, ping, discovery and
nine tools: work_create, work_list, work_run, work_attempts, work_cancel, exchange_publish, exchange_read,
exchange_submit, note_publish. The upstream target is compiled in:
`http://127.0.0.1:18085/project-mcp`. No guest-selected host is accepted.
The Compose Firecracker overlay publishes MCP there on host loopback.

Install `/etc/agent-workbench/firecracker/project_mcp_signing_key` as a root-owned
0600 file, with root-owned non-writable ancestors. Its content must equal the MCP
and control-plane `CONTROL_TOKEN_FILE` signing authority. This is deliberately a
host-only file; neither that key nor a global MCP/operator credential enters the
guest. Missing provisioning prevents Hermes admission and capability advertisement.
A mismatch with upstream is rejected during discovery, before model execution.
Rebuild and restage the immutable image: it now includes both frozen `matrix` and
`mcp` extras. Port 18085 must be allowed in MCP_ALLOWED_HOSTS (the existing
`127.0.0.1` hostname entry allows this explicit port).

Task and Matrix adapters write a private generation-grant file and supply
WORKBENCH_MCP_URL=http://127.0.0.1:18085/mcp,
WORKBENCH_MCP_TOKEN_FILE and WORKBENCH_PROJECT_ID. Hermes config uses
mcp_servers.workbench.url, headers.Authorization and tools.include. Explicit
pinned discovery registers mcp-workbench; a failed discovery fails the adapter.
Every broker request checks VM identity/running/stopping state and its generation
grant. Each accepted request receives a new host-only wpa1 credential expiring in
60 seconds; there is no guest renewal endpoint or reusable project token in the
guest. Stop/replacement revokes the channel immediately for subsequent requests.
Already accepted operations retain their ordinary API idempotency semantics.

`HERMES_SOURCE=/path/to/pinned/source go test ./internal/runtime/firecracker -run
TestProjectMCP -v` runs real Hermes discovery/filtering/registry invocation through
the broker and real Go MCP ingress. The fixture control-plane responds with a
retryable failure, rejects foreign project scope, and tests generation revocation
and expired application credentials. It does not claim Firecracker boot or a live
provider/account test.

### Digest fixture (one Workbench owner)

Stage fixtures/digest-task.md and fixtures/digest-source.json into the owning
Hermes guest's `files/jobs/` using the normal guest file transfer path. Substitute
project/workspace/time in fixtures/digest-schedule.json, then POST that JSON to
`/v1/schedules` using operator/project authorization. The existing scheduler
accepts provider=hermes for Hermes workspaces; its native task runs the deterministic
`/usr/local/bin/hermes-digest` command inside the Hermes workspace. The command
uses the same generation-scoped broker and the pinned MCP 2.x SDK, publishes a
sourced note through note-publications, and optionally accepts --request-export
for an explicitly granted exchange request. The command defaults its occurrence to WORKBENCH_TASK_ID, supplied by the
native adapter. Retrying an accepted task returns the same receipt; each newly
scheduled task has a distinct occurrence. No Hermes cron is installed. The notes
binding must already be enabled via POST /v1/projects/{id}/coordination.

Remaining acceptance: actually register and execute this schedule on a booted,
staged Firecracker host with a configured native Codex subscription; verify its
run receipt, guest note and retry receipt. Recurring occurrences use the same task-ID propagation (the supplied fixture
remains a single scheduled run). No external research collection, account access, Matrix live
session, or Obsidian live vault/sync acceptance is claimed by these local tests.
