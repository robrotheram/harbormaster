# Project workbench

Open http://127.0.0.1:8080/app/. The UI uses React, TanStack Router and TanStack Query. All assets and fonts are bundled locally.

## Start working

1. Create a project in the project selector. The selected project stays visible throughout the UI.
2. Create a workspace. Choose T3 for interactive native browser/desktop work, or Codex for task execution. Each new workspace has a private persistent agent home.
3. Open T3 or Connect T3 desktop, or submit a workspace task. Native desktop pairing continues to use the same workspace files and conversations.
4. For a PR workspace, branch from an idle workspace in the same project. This copies code, not conversation or agent-home state.

Existing workspaces remain in **Workspaces**, project `default`. Their storage and native histories are preserved. This grouping is not a claim that the earlier use-case names were separate legal projects. Create explicit projects for new isolated work; there is no automatic content reclassification.

## Schedule work

Create an instruction file inside the workspace, for example `TASK.md`. In Schedules, choose the workspace and that relative path, a first run time and optional recurrence. The minimum recurrence interval is sixty seconds. Instructions stay in workspace storage; the platform persists the path, schedule and operational run identifiers only.

Run now creates an occurrence using a durable idempotency key. The scheduler starts a stopped workspace and submits the file through its native durable acceptance endpoint. Lost acknowledgements retry the same occurrence; file receipt deduplication returns the already accepted task even if the instruction file later changes. Interrupted native tasks are not automatically replayed.

Disabling a schedule prevents future automatic occurrences; it does not cancel already accepted tasks. Missed recurring intervals are coalesced into one occurrence rather than replayed as a burst. A busy workspace or exhausted compute quota leaves work queued. No API-key billing fallback is selected.

## Connect an external agent

Issue a project credential from the local human operator terminal:

```sh
go run ./cmd/workbench project list
go run ./cmd/workbench project access PROJECT_ID
```

The command writes a private, expiring MCP configuration under `.local/project-access/` and prints its path, never its credential. Configure the corresponding project's T3/agent environment using that file. The MCP endpoint is http://127.0.0.1:8082/mcp. Project credentials cannot create projects or operate other projects. The original bootstrap MCP credential is now restricted to the legacy `default` project instead of receiving global control authority.

The MCP tools include project/home discovery, workspace creation and lifecycle, native task submission/results/cancellation, and schedule creation/listing/actions/run metadata. The shared human desktop can use several project connections; an agent connection carries only its authorized project.

## Sharing

File handoffs and branch baselines require the same project on both sides, including requests made by the human operator. Existing files are never overwritten by handoff. There is no cross-project export feature. Workspace conversations, agent memories and home directories are private by default.

## Deployment status

Compose remains the functional development deployment with unverified VM isolation and full network containment. Native T3 links do not themselves provide project isolation; do not configure cross-project peer links. The production runtime configuration and network policy must enforce that boundary outside agents.

The privileged runtime controller also has a Kubernetes backend using pinned Kubernetes SIGs Agent Sandbox resources. See `deploy/kubernetes/README.md` for preprovisioned project namespaces, required Kata RuntimeClass, approved image digests and policy/RBAC installation. Cluster access is not configured by the UI and cannot be supplied by an agent. No live EKS/k3s or Kata verification is implied by unit tests.

Hermes/Matrix deployment material is under `deploy/hermes`. A Matrix homeserver, room, bot identity/crypto state and a supported subscription login must be configured before it can receive messages. Hermes availability is reported honestly; selecting a record does not manufacture a working integration. Claude remains deferred until its native login and permitted deployment path are configured. The application never converts a failed subscription login into API billing.

## Verification

```sh
go test -race -timeout 90s ./...
go vet ./...
cd frontend
npm ci
npm test
npm run build
```

The local operator UI remains loopback-only and does not implement multi-user login. Cluster/remote access requires an authenticated ingress and project-bound credentials; never publish the unauthenticated human launcher directly.

All UI pages now live in React, including Runtime overview and Getting started. The old static launcher/dashboard assets are removed. On a T3 workspace, Connect T3 desktop issues a fresh one-use native pairing URL; Open T3 desktop launches the installed app through its `t3code://pair` link. Copy pairing URL remains available as a fallback. Old page URLs redirect into React.

Codex task workers can also host the native T3 interface. Select **Enable T3** on an idle worker, then use **Open T3** or **Connect T3 desktop**. This adds an interactive console without changing project/home ownership or removing task history. The console setting persists in the private home across restarts. Older stopped runtime containers may need an operator image refresh before enabling this capability.
