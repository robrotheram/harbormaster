+++
title = "Hermes workspace and Matrix connection"
aliases = ["/docs/reference/deploy/hermes/readme/"]
+++

The [runtime manifest](../../../../workspace/hermes-runtime.json) pins Hermes v0.23.0 and its source checksum. This guide covers the Firecracker worker and its Matrix connection. For Kubernetes without Matrix, follow the [Kubernetes Hermes guide](../k3s-hermes.md). The guest build uses upstream's frozen
`uv.lock`, including Matrix encryption dependencies. The host installation and a
real VM import check must pass before Hermes appears as available in the UI.

## Use the workspace agent

1. Open a project. Select **Settings**, then **Hermes setup**.
2. In **Agent**, choose **Workspace agent**, enter an approved model, and choose an approved connection profile. To enable both workspace and Matrix use, choose **Workspace agent and Matrix bot** and complete the fields in **Matrix** before saving.
3. Select **Save settings**, then **Start Hermes**.
4. Open **Workspace**, then **Project assistant**. Select **Work with Hermes** and submit an instruction.

Provider requests use
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

1. In **Hermes setup**, open **Matrix**. Enter the HTTPS homeserver URL, bot user ID, exact room IDs
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

Follow the [Firecracker installation steps](../firecracker.md) or the
[Kubernetes Hermes guide](../k3s-hermes.md). The workspace image contains
the pinned Hermes engine and performs an import check during the build. Stop
workspaces cleanly and stop the Firecracker host controller before replacing its
artifacts.

Sources: [pinned upstream source](https://github.com/NousResearch/hermes-agent/tree/2bd378ed1b8d6bf2192cb26e0a55ff3091e6bd19),
[Matrix adapter](https://github.com/NousResearch/hermes-agent/blob/2bd378ed1b8d6bf2192cb26e0a55ff3091e6bd19/plugins/platforms/matrix/adapter.py).

## Adapter maintenance

Follow the [Hermes package guide](../development/hermes.md) for adapter changes and tests. After you rebuild and stage the guest image, run `hermes-runner --version` in a booted worker to verify imports against the pinned SDK.

### Fixed project MCP channel

Firecracker Hermes guests use loopback port 18085, bridged over vsock to the
VM-specific host listener. Only `POST /mcp` is accepted, with the current workspace
generation grant. The host forwards only MCP initialization, ping, discovery and
the tools in the [shared project tool list](../../../../internal/shared/project-tools.json). The upstream target is compiled in:
`http://127.0.0.1:18085/project-mcp`. No guest-selected host is accepted.
The Compose Firecracker overlay publishes MCP there on host loopback. Set
`FIRECRACKER_HERMES_PROJECT_MCP=true` on the host controller (including during
Hermes self-test) to activate the channel. Each VM gets an independent limit
of 8 requests per second with a burst of 16.

During Firecracker provisioning, copy `.local/secrets/project_signing_key` to
`/etc/agent-workbench/firecracker/project_mcp_signing_key`.
It is a root-owned 0600 file with root-owned non-writable ancestors. Its content must equal the MCP
and control-plane `PROJECT_SIGNING_KEY_FILE` signing authority. This is deliberately a
host-only file; neither that key nor a global MCP/operator credential enters the
guest. Missing provisioning prevents Hermes admission and capability advertisement.
A mismatch with upstream is rejected during discovery, before model execution.
The guest image includes the frozen `matrix` and `mcp` extras. Include `127.0.0.1` in `MCP_ALLOWED_HOSTS` for the host listener.

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

Stage `digest-task.md` and `digest-source.json` from
`workspace/hermes/examples/` into the owning Hermes guest's `files/jobs/`.
Substitute project, workspace and time in `digest-schedule.json` from the same
directory, then POST that JSON to
`/v1/schedules` using operator/project authorization. The existing scheduler
accepts provider=hermes for Hermes workspaces; its native task runs the deterministic
`/usr/local/bin/hermes-digest` command inside the Hermes workspace. The command
uses the same generation-scoped broker and the pinned MCP 2.x SDK, publishes a
sourced note through note-publications, and optionally accepts `--request-grant`
for an explicitly granted exchange request. The command defaults its occurrence to WORKBENCH_TASK_ID, supplied by the
native adapter. Retrying an accepted task returns the same receipt; each newly
scheduled task has a distinct occurrence. No Hermes cron is installed. The notes
binding must already be enabled via POST /v1/projects/{id}/coordination.

Remaining acceptance: actually register and execute this schedule on a booted,
staged Firecracker host with a configured native Codex subscription; verify its
run receipt, guest note and retry receipt. Recurring occurrences use the same task-ID propagation (the supplied fixture
remains a single scheduled run). No external research collection, account access, Matrix live
session, or Obsidian live vault/sync acceptance is claimed by these local tests.
