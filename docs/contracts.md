# MVP service contracts

Implementation baseline; all JSON uses snake_case. The Go module is `agent-workbench`.

## Control API

All routes require `Authorization: Bearer <provisioned local control token>`. Initial deployment is one explicitly configured local principal (`local-operator`) and project (`default`), not multi-user OIDC. The token is file-provisioned, never a caller-supplied identity. Dashboard has a separate unauthenticated read-only listener.

- `GET /healthz`: `{"status":"ok"}` (public health only).
- `GET /v1/workspaces`: `{"items":[Workspace]}`.
- `GET /v1/workspaces/{workspace_id}`: Workspace.
- `POST /v1/workspaces`: `{ "project_id":"default", "idempotency_key":"...", "name":"...", "parent_id":"...", "branch":"..." }` → Workspace (201, or 200 on replay). Name/parent/branch are optional; a parent requires a new branch. The source must be idle.
- `POST /v1/workspaces/{id}/start`: `{ "idempotency_key":"..." }` → Workspace. Synchronous bounded readiness (up to 30s), subsequent retries attach to the same runtime. MCP should use a bounded HTTP timeout and report NOT_READY/UNAVAILABLE safely.
- `POST /v1/workspaces/{id}/stop`: `{ "idempotency_key":"..." }` → Workspace; reject busy or unknown activity. Storage persists.
- `POST /v1/workspaces/{id}/tasks`: `{ "instruction":"...", "idempotency_key":"...", "provider":"codex|claude" }` → Task (202). Starts workspace as needed. Provider is optional and otherwise uses the container's configured default; a fixture default retains the original built-in runner. Changing provider with an accepted task key conflicts.
- `GET /v1/workspaces/{id}/tasks/{task_id}` → Task metadata.
- `GET /v1/workspaces/{id}/tasks/{task_id}/result?offset=0&limit=65536` → `{task_id,state,content,next_offset}`. `next_offset=-1` marks the end of the current result; otherwise it is the next byte offset. Content forwarded from workspace; never persisted in platform database.
- `POST /v1/workspaces/{id}/tasks/{task_id}/cancel`: `{ "idempotency_key":"..." }` → Task.
- `GET /v1/usage`: current aggregate quota and usage, no historical data.

Workspace: `{id,project_id,state,ready,active_runs,background_jobs,created_at,updated_at,idle_stop_at,runtime_type,isolation_status,network_status,cpu_limit,memory_limit_bytes,cpu_cores,memory_bytes,observed_at}`. Nullable measurements are unknown, never fabricated zeros. State: STOPPED, STARTING, READY, BUSY, STOPPING, ERROR, UNKNOWN. runtime_type: docker or kata. Isolation/network: unverified, verified, failed. No image/path/identity parameters accepted from callers. Workspace metadata also carries optional `name`, `parent_id` and `branch` fields. Legacy stored records may still carry `profile_id`; it is not accepted when creating a workspace and no longer configures tools.

Task: `{id,workspace_id,state,created_at,updated_at,error_code?}`. States queued,running,cancel_requested,succeeded,failed,cancelled. Task results and instructions exist only in workspace SQLite. Control-plane metadata mirrors status but must refresh from workspace; unreachable workspaces produce explicit unavailable responses.

Errors: `{code,message,retryable}` with no internal details. HTTP 400 INVALID_REQUEST, 401 UNAUTHORIZED, 404 NOT_FOUND_OR_FORBIDDEN, 409 WORKSPACE_BUSY/IDEMPOTENCY_CONFLICT, 429 QUOTA_EXCEEDED, 503 NOT_READY/RUNTIME_UNAVAILABLE/AGENT_PROTOCOL_UNAVAILABLE.

## Dashboard

Separate container; no authentication; same-origin proxy for exactly `GET /dashboard-api/v1/snapshot` to `http://controlplane:8081/dashboard-api/v1/snapshot`. No generic proxy. Default host publication `127.0.0.1:8080`.

Snapshot: `{schema_version:1,generated_at,mode:"development",workspaces:{observed_at,status,items:[Workspace]},active_tasks:{observed_at,status,items:[Task]},usage:{observed_at,status,cpu_allocated,memory_allocated_bytes,running_workspaces,max_running_workspaces,cpu_quota,memory_quota_bytes},services:{observed_at,status,items:[{name,status,reason_code?,observed_at}]}}`. Status ok/partial/unavailable; service healthy/degraded/unavailable/unknown/not_configured. Never expose instructions, results, labels, paths, URLs, secrets or raw error text.

The interactive launcher at `/workspaces` is a separate local-operator surface with a provisioned control token. Its `/ui-api/` adapter permits profile/workspace listing, creation, start/stop, desktop pairing and bounded file handoffs. Browser mutations require JSON, a custom request header and matching Origin. It has no Docker or Vault authority.

Open T3 mints a one-use, one-minute connection ticket after workspace start. The fragment-only ticket is exchanged on that workspace's own `ws-<id>.localhost` origin for a twelve-hour host-only HttpOnly SameSite cookie. Every native HTTP/WebSocket request validates the workspace-bound cookie. The proxy chain carries native protocol 2 to the managed workspace, replacing each internal hop's authorization. No privileged native browser-session bearer or platform control credential is returned to the browser. See [interactive workspace access](interactive-workspaces.md).

`POST /ui-api/workspaces/{id}/desktop-pair` accepts a lifecycle idempotency key, starts the workspace, then requests a fresh native T3 pairing credential. Each call mints a fresh credential even when the start key repeats. The response is `{pairing_url,expires_at}` with a fragment-only token and `Cache-Control: no-store`. Links are issued by T3 and expire according to its policy. Native `desktop-ws-<id>.localhost` requests follow a separate authenticated management path to `/v1/t3-desktop/{path...}`. The ingress overwrites `X-Workbench-Native-Authorization` with the original client Authorization header. The supervisor removes that header, restores native client authority, and never injects its browser token. T3 verifies pairing/token exchange, client credentials, DPoP and WebSocket tickets. Only matching HTTP Origins or the exact installed native renderer Origins are accepted. These routes remain loopback-only.

`POST /v1/file-handoffs` (also available through `/ui-api/file-handoffs`) takes `{source_id,source_path,destination_id,destination_path}`. Both workspaces are authorized before observation or content access, must be running/ready/idle, and remain separate volumes. The source file is read through confined `POST /v1/files/read`; the destination uses `POST /v1/files/write-new`. Content is base64-encoded JSON internally, bounded to 1 MiB, and never persisted by platform services. Writes stage a complete file and publish it with an exclusive hard link; existing paths return `DESTINATION_EXISTS`. Paths are project-relative, excluding Git metadata and named credential/configuration paths. A successful handoff returns HTTP 201 `{copied:true,bytes}`. Handoffs do not have exactly-once retry semantics; a lost response requires checking the destination before retrying.

## Workspace and runtime controller

Runtime management endpoints authenticated with dedicated controller token. Controller alone has Docker API access. Workspace bearer token derived per workspace, distinct from control/controller tokens; inference/broker use that token to bind identity through runtime registration. Workspace exposes `/healthz`, `/v1/activity`, `/v1/tasks` (POST/GET), `/v1/tasks/{id}` (GET), `/v1/tasks/{id}/result`, `/v1/tasks/{id}/cancel` and the two bounded file-handoff endpoints. Runtime controller forwards only those approved paths through `/v1/workspaces/{id}/agent/...`.

## Gateways

Two separate Go binaries. Ports 8080. `GET /healthz` public. No generic proxy or CONNECT.

- Inference: `POST /v1/chat/completions`, OpenAI-compatible messages/tools/model, response unchanged (nonstreaming initially). Explicit allowlisted origin/model from file. `GATEWAY_MODE=fixture` provides visibly labeled deterministic synthetic inference. `GATEWAY_MODE=upstream` uses fixed configured HTTPS origin (no client URL). Streaming optional but if unsupported reject explicitly.
- Broker: `POST /v1/operations/{operation}` with validated named parameters; fixture mode returns synthetic data. Upstream fixed origins, operation templates and certificate references in file config. Vault holds certificate/private key; no arbitrary URL/Vault path supplied by client.
- Both authenticate workspace tokens by calling trusted controller `POST /v1/grants/verify` using gateway service credential (`GATEWAY_SERVICE_TOKEN_FILE`). Request `{token,audience}` audience inference or broker. Response `{workspace_id,project_id,principal_id}` on success; fail closed on errors. Controller keeps workspace grants in memory derived from stable secret, binds to current running generation; no access when stopped. Tests may inject a verifier function. Gateways configuration may pass trusted URLs only via env/file, never request fields. Controller URL `http://runtime-controller:8080`.
- Environment: `GATEWAY_MODE`, `GATEWAY_CONFIG` (JSON policy), `CONTROLLER_URL`, `GATEWAY_SERVICE_TOKEN_FILE`; inference upstream credential `INFERENCE_API_KEY_FILE`; broker `VAULT_ADDR`, `VAULT_TOKEN_FILE`. Credential files supplied by operator. No generated fake credentials representing external services.

## MCP

Separate Go binary with official pinned Go SDK, Streamable HTTP `/mcp` and health route. Environment `CONTROLPLANE_URL=http://controlplane:8080`, `CONTROL_TOKEN_FILE`, `MCP_TOKEN_FILE` (distinct caller-facing token), `MCP_ALLOWED_HOSTS` default localhost,127.0.0.1,mcp. Forward only fixed API routes above; reject arbitrary IDs containing path separators. Enforce Host/Origin. Read tokens from files; no baked credentials. Authenticated even locally; dashboard alone is exempt. Avoid async MCP extension; ordinary tools return application task IDs. Tests with httptest upstream and official MCP client if feasible.

## Native T3 workspace process

Interactive containers run a pinned native T3 server/client bundle beside the Go workspace supervisor. Readiness includes native session creation and a valid protocol-2 shell snapshot. Activity combines native T3 active/background thread state with process observation. Idle T3, resource-monitor, Codex app-server and local MCP protocol processes are recognized individually; arbitrary terminals/commands still block idle-stop. A native T3 failure reports unknown activity and blocks automatic stopping.

Only the stale `userdata/server-runtime.json` process-discovery record is removed at container startup; environment identity, SQLite files and native sessions are retained. Branch seeding copies only `/data/files`, freezes/resumes an idle source, and requires a completion marker before starting the target. Transfer limit: 256 MiB. It never copies the parent's supervisor/task or native T3 databases.

An idempotent start of an already-ready workspace also reconciles platform network attachments. The authenticated controller `POST /v1/workspaces/{id}/reconcile-network` operation reattaches the deployment-owned controller, gateway and broker only; it does not restart a workspace or accept caller-selected networks. This restores connectivity after a platform container is recreated.

## Workspace settings

`POST /v1/workspaces/{id}/settings` accepts exactly
`{idle_timeout_minutes: 0 | integer(5..1440), github_access: boolean}`. Project
authorization applies before observation or mutation. Zero disables automatic
shutdown; omitted metadata uses the 60-minute deployment default. An explicit
open/start renews `last_opened_at`; polling does not. Transition from BUSY to READY
begins a full idle interval. Unknown activity always blocks shutdown.

The trusted controller accepts `POST /v1/workspaces/{id}/network-policy` with
`{connections:{mode,destinations}}` (legacy `{github_access:boolean}` remains supported) using its management credential only. Its policy is
bound to the observed container generation and defaults to deny after restart.
An `egress` grant is issued/verified only for an enabled, running generation.
The dedicated egress service accepts HTTP and CONNECT only to destinations in the workspace policy;
DNS resolution and public-address validation happen there. The supervisor exposes
a credential-free loopback proxy to native tools and injects its workspace grant
on the internal hop. It never gives Git a provider/Vault credential. Active
tunnels revalidate authority every five seconds with a five-second verifier
timeout. Destination approval covers the encrypted host, not HTTP paths.

`git_proxy_ready` reports whether the container includes this proxy setup; it
is separate from VM/network isolation evidence. Existing running containers stay
intact. On their next stop/start, old stopped containers are recreated with the
same validated volume, project and private home, without volume deletion.

Workspace settings also accept `connections:{mode:"blocked"|"allowlist"|"open",
destinations:string[]}` instead of the legacy GitHub field. Up to 64 exact,
lowercase `host:port` destinations are accepted in allowlist mode only. Changing
direct networking on a running workspace returns `STOP_WORKSPACE_FIRST` without
saving the change. Mode/proxy-version observations distinguish actual runtime
configuration from desired settings. Proxy capability version 2 supports custom
HTTP/HTTPS destinations. Open mode enables direct egress/DNS and skips proxy env.

## Global connection profiles

`GET /v1/connection-profiles` returns `{items:[{id,name,description,policy,
revision,updated_at}]}`. The authenticated local operator can create profiles via
`POST /v1/connection-profiles` (revision 0) and edit via
`POST /v1/connection-profiles/{id}` (expected current revision). Stale edits return
`PROFILE_REVISION_CONFLICT`; project credentials cannot mutate global profiles.
Names are bounded to 80 bytes, descriptions to 500, policies use existing rules.

Profiles persist in PostgreSQL metadata, seeded once without replacing edits.
Workspace creation/settings accept `connection_profile_id`; workspace metadata
stores `connection_profile_revision` plus an applied `connections` snapshot.
Explicit starts/opens or saving workspace settings resolve the latest version.
Internal access to an already-running workspace retains its snapshot; an actual
cold start resolves current policy. Desired direct-network changes on running
workspaces are refused without overwriting the applied snapshot. Workspace-local
connections detach the global reference. Profile configuration contains no
workspace data, conversation content or credential material.

## Application and runtime compatibility

`POST /v1/projects/{id}/hermes/assistant` accepts `{idempotency_key}` and owns
project assistant selection, creation, connection-profile application and startup.
It returns the started Workspace (201 when created, 200 when reused). Multiple
candidate Hermes workspaces return 409 `ASSISTANT_WORKSPACE_AMBIGUOUS`; callers
can choose a workspace explicitly through the normal inventory/lifecycle routes.
The local operator ingress exposes this exact operation under `/ui-api`.

Task status and result polling do not renew `last_opened_at`. For compatibility,
a cold task read can wake stopped compute to access its private task database;
this availability operation does not count as an explicit user-open. An explicit
start/open renews the idle interval. Unknown or unavailable
activity remains conservative and blocks automatic stop.

Runtime `GET /v1/capabilities` returns a flat typed capability object including
`schema_version`, `backend`, `id`, `version`, `agent_kinds`, `network_modes`,
`branch_seed`, `private_homes`, `isolation_status`, `network_status`, and
`resources:{cpu_cores,memory_bytes}`. The internal start specification carries
the reserved resource limits; an incompatible template returns
`RESOURCE_LIMIT_UNAVAILABLE` before allocation. Empty
`network_modes` means no network policy is currently supported, not implicit
permission to bypass policy. The control plane rejects unsupported startup
policies before allocating compute. Kubernetes remains experimental until its
network policy integration is implemented and accepted.

Controller workspace forwarding validates queries by endpoint. History requests
on `GET /v1/tasks` accept the bounded history selector, paging cursors, ordering,
episode and limit. Task result requests accept nonnegative offset/limit only.
Arbitrary or repeated query fields are rejected consistently by every backend.

Metadata schemas migrate before startup, never from a request or scheduling tick.
Scheduled occurrence insertion and next-trigger advancement commit together.
Pending traversal progresses past unavailable work; acknowledgement loss retries
the same workspace acceptance key. The supported deployment has one
control-plane owner; multiple replicas require additional durable coordination.

Tool profiles are removed. `/v1/profiles` and `/v1/tool-profiles` are unavailable;
workspace creation does not accept `profile_id`. T3 and native Codex/Claude
workers use fixed bundled file/Git MCP servers. No caller-selected tool lists
are passed through runtime specifications or workspace environment variables.
Existing workspace/home ownership and legacy private storage paths are retained.
