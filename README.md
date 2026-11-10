<img src="assets/icon.svg" width="64" height="64" alt="">

# Agent Workbench

**Project workbench:** open [the TanStack React UI](http://127.0.0.1:8080/app/). [Project guide](docs/project-workbench.md) covers private agent homes, workspace/PR orchestration, schedules and project-scoped MCP credentials. Existing data stays in the labelled legacy project.

Local primitives for an agent-operated workbench. Go services, Docker Compose, persistent workspaces, a T3 workspace launcher, an operational dashboard and an external-agent MCP interface.

**New here?** Follow [Your first session](docs/getting-started.md) for a copy-and-paste walkthrough, expected results, MCP access and troubleshooting. The running dashboard also has a **Getting started** link.

**Next development:** [Software factory and research delivery plan](docs/software-factory-plan.md) covers GoGallery, experimental research, personal/project Hermes assistants, Compose development, and k3s production.

**Ready for real coding?** [Codex and Claude subscription setup](docs/subscription-agents.md) runs the native agent inside a workspace while Vault and the inference gateway keep provider credentials outside it.

**Use Hermes:** select a project → **Hermes setup** to configure its sandboxed
workspace agent and an encrypted Matrix bot. [Hermes connection guide](deploy/hermes/README.md)
covers starting the runtime and saving the host-held bot credential.

**Develop in the browser:** open [the workspace launcher](http://127.0.0.1:8080/workspaces), create a project workspace and select **Open T3**. [Interactive workspace guide](docs/interactive-workspaces.md) covers bundled workspace tools and separate branch workspaces.

**Use T3 desktop or share files:** select **Connect desktop** and paste its one-use URL into desktop **Settings → Environments → Add environment**. The launcher also has **Share a file between workspaces** for explicit copies into another workspace. [Connection and sharing guide](docs/interactive-workspaces.md#connect-t3-code-desktop).

**Development only:** the development deployment uses synthetic inference/data fixtures and ordinary Docker containers. VM isolation and complete host-level egress containment remain unverified. See [security boundaries](docs/security.md). Kubernetes is an experimental Agent Sandbox adapter; unsupported network policy fails closed before allocation. See [backend support](docs/architecture.md#backend-support).

**Firecracker sandbox:** [installation and acceptance checks](docs/firecracker.md)
cover jailed microVMs with no network card and fixed gateway channels. This mode
refuses Fully open networking and Docker fallback. Host installation and real VM
acceptance must pass before restarting the workbench.

## Start locally

Requires Docker Engine with Compose, access to its daemon, Python 3 and OpenSSL for bootstrap, and free localhost ports 8080, 8082 and 8090. Go 1.27.3 is required for native development/tests.

```sh
make up
```

This provisions random local application credentials without printing them, builds every service plus the workspace image, and starts the platform. Existing credentials and persistent data are preserved. The dashboard is at **http://127.0.0.1:8080** and requires no login. MCP is at **http://127.0.0.1:8082/mcp**. The management API is at **http://127.0.0.1:8090**.

Only these loopback ports are published. PostgreSQL, Vault and the runtime controller have no published host ports. `make down` stops the platform without removing volumes. Dynamically created workspaces are managed separately; stop them before taking the platform down.

## Exercise a workspace

```sh
python3 scripts/client.py create --key first-workspace
python3 scripts/client.py list
python3 scripts/client.py submit WORKSPACE_ID 'Describe the current workspace.' --key first-task
python3 scripts/client.py status WORKSPACE_ID TASK_ID
python3 scripts/client.py result WORKSPACE_ID TASK_ID
python3 scripts/client.py stop WORKSPACE_ID --key stop-workspace
```

Replace the returned opaque IDs. Reuse a mutation key to safely retry the same request. Changing task instructions with a previously accepted task key returns a conflict. Fixture responses are synthetic and do not represent a real model/provider call.

**Agent activity:** open **Workspaces → Activity logs** for live task, tool and background-service events across the project. Logs remain available after stop/restart. [Activity logging guide](docs/workspace-logging.md).

Accepted tasks survive client disconnect. Files, messages and results live in the workspace's SQLite/database volume. A workspace stops after 60 minutes by default without runs or background processes. Stop/restart retains storage; interrupted processes are recorded as interrupted, never silently replayed. The built-in runner command tool has a 30-second limit; native subscription CLI tasks have a ten-minute overall limit; detached background commands are unsupported and detected processes prevent idle shutdown.

## Components

| Component | Responsibility |
| --- | --- |
| Control plane | Single local principal, metadata, quotas, lifecycle and task routing |
| Runtime controller | Fixed Docker/Kata execution profiles and generation-bound workspace grants |
| Workspace | SQLite tasks/conversation records, files and bounded coding tools |
| Dashboard / launcher | Local single-operator workspace controls, native T3 access and read-only runtime overview |
| MCP | Separate authenticated adapter for external agents |
| Inference gateway | Approved inference models and destinations |
| Credential broker | Named data operations using broker-held mTLS credentials |
| PostgreSQL | Platform metadata only |
| Vault | Persistent certificate store; real deployment initialization is operator-managed |

The first local principal is `local-operator` in project `default`. This does not implement multi-user OIDC or claim multi-user access isolation. The loopback launcher requires no account login; it can control workspaces and issues workspace-specific browser sessions. Treat access to that local UI as operator access.

## MCP

Configure an HTTP MCP client for `http://127.0.0.1:8082/mcp` with an Authorization bearer header using the credential in `.local/secrets/mcp_token`. It differs from the internal control token; never put either in a URL or commit it. The server checks Host/Origin and exposes typed workspace lifecycle, task submission/status/result/cancellation, and current usage tools.

For another machine, use an authenticated SSH tunnel to the loopback endpoint rather than publishing unauthenticated network access. Client compatibility must be tested against the intended client/version. Results may contain sensitive workspace content; result access conveys that content to the caller.

## Real integrations and Kata

Keep fixture mode enabled until upstream policies and credentials are provisioned. Gateway configuration lives in `config/`. Vault starts as a persistent, uninitialized service with no root token invented or embedded by the application. Initialize/unseal it and provide a scoped broker policy before enabling real certificate requests. No live inference or user certificate is required for fixture tests.

Set `WORKSPACE_RUNTIME=kata` only after installing and configuring a compatible Kata shim on the Docker host. The controller requests `io.containerd.kata.v2` and does not fall back. `REQUIRE_VERIFIED_ISOLATION=true` currently blocks startup because this code has no verifier that can establish the required isolation/network evidence.

Interactive workspaces run the pinned native T3 server and bundled web client. Its protocol-2 WebSocket traffic is proxied without a Go wire reimplementation. Use **Open T3** for browser access or **Connect desktop** for native desktop pairing. `/v1/workspaces/{id}/connection` remains the legacy, unimplemented generic attachment contract. The task API/MCP remains available separately.

## Development and checks

See [CONTRIBUTING](CONTRIBUTING.md) for reproducible checks and [architecture](docs/architecture.md) for module boundaries. The project uses the [MIT license](LICENSE).

```sh
make check
make browser-check  # install Chromium first; see CONTRIBUTING.md
docker compose config --quiet
make smoke  # after make up; requires fixture mode and Docker daemon access
```

Tests cover lifecycle races, durable deduplication, cancellation, content boundaries, coding tools, gateway policies, MCP SDK interoperability and dashboard read-only behavior. Tests use SQLite for platform-metadata fixtures; the Compose deployment uses PostgreSQL. Docker-level tests require a reachable Docker daemon and do not run implicitly in the unit suite.

The application contracts and remaining decisions are in [docs/contracts.md](docs/contracts.md). Generated credentials and local state are ignored by git. Never use `docker compose down --volumes` unless you intend to delete platform metadata and Vault data.

Configure per-workspace **Connection profile** under **Workspaces → Settings**:
Blocked, GitHub only, Custom allowlist (exact public `host:port` entries through
the HTTP/HTTPS proxy), or Fully open (direct networking and DNS). Stop before
switching into/out of Fully open. Files and private homes are preserved. See
[workspace connection profiles](docs/getting-started.md#workspace-connection-profiles).

Global profiles are editable at [Connection profiles](http://localhost:8080/app/connection-profiles).
Select a reusable profile in workspace creation/settings. Global edits are
versioned; linked workspaces show pending changes and adopt them on explicit
open/start or settings save. Existing workspaces retain their current policy.

See [private message history and JSONL evaluation exports](docs/workspace-history.md) for retained conversations, tool interactions and metadata.
