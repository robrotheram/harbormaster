# Workbench frontend

React with TanStack Router and Query, Tailwind CSS v4, and checked-in Radix-backed UI primitives. Fonts and icons are bundled locally.

## Develop and check

```sh
npm ci
npm run dev
npm test
npm run typecheck
npm run build
npm run format:check
```

Development serves `/app/` and proxies API requests to the existing dashboard at `http://127.0.0.1:8080`. Set `WORKBENCH_PROXY_TARGET` to use another trusted development dashboard. The proxy preserves the browser Host so the ingress can enforce its matching Origin rule; production ingress checks are unchanged.

The browser suite runs against a synthetic local host and needs no Docker deployment, provider credentials, T3 installation, or machine-specific browser path:

```sh
npm run test:browser:install
npm run test:browser
npm run test:connections
```

On a fresh Linux CI image, install browser system dependencies with `npx playwright install --with-deps chromium`. The suite builds the current frontend and starts its compiled bundle with the dashboard’s production content security policy, plus a synthetic ingress on loopback ports 4173 and 4174. It uses the same proxy configuration as development, without HMR or dependency-optimizer reloads. It covers protected proxy mutations and rejected foreign origins, project switching, workspace creation, pairing, handoffs, removed scheduling navigation, task cancellation and escaped results, settings, theme/focus/mobile behavior, and the single backend assistant-start operation. Application fixtures are synthetic; this does not replace live runtime or production ingress acceptance tests. Browser artifacts are written to `artifacts/`.

The separate connection-page suite requires the Go toolchain. It starts the real
dashboard ingress with a synthetic control plane on loopback port 5191 and checks
preview/worker connection recovery, one-use tickets, CSP, responsive layouts and
accessibility. It does not connect to live workers or provider accounts.

## Factory navigation

`/app/` opens **Projects**. Project routes include **Overview** for tasks, **Workspace** for agent threads and previews, **Reviews** for decisions, **Workers** for lifecycle and history, and **Settings** for project configuration. **Project assistant** in **Workspace** uses the backend assistant-start and task APIs. See the [user guide](../docs/user-guide.md).

`/app/infrastructure` is deployment-wide. It combines authorized project worker
inventories with resource allocations from the platform snapshot, and displays
the actual reported runtime. Open Bulk operations to select individual workers
or all visible rows and start, stop or delete a batch. Selection has its own narrow
column and stays hidden during normal browsing. Done exits bulk mode; exiting or
changing a filter clears the selection. Each batch
confirms its targets, limits concurrent requests, reports results per worker, and
retries only failures. The existing backend lifecycle rules still apply.
`/app/platform-services` opens from **Platform**, under **Health**. **Platform** also includes **Connections** and **Network profiles**. **Docs** opens `/app/help`.
Unavailable project observations are explicitly partial; runtime names are not
claims of verified isolation. The old project `/overview` bookmarks render the
global infrastructure view as well.

`tests/software-factory.spec.ts` covers project scoping, queue filtering, Hermes
entry, global runtime/project filters, missing observations, and rendered layouts
at desktop and mobile widths. Its screenshots use synthetic data, not live load.
`tests/bulk-workers.spec.ts` covers lifecycle batches, partial failures, retries,
delete confirmation and selection boundaries.
`tests/ui-consistency.spec.ts` checks every main page at desktop and narrow mobile
widths, including heading structure, overflow, aligned compact controls and table
gutters.

## Ownership and dependencies

- `src/app/`: bootstrap, routing, providers and application shell. Feature screens load lazily.
- `src/features/workspaces/`: inventory, workspace creation/settings, native attachment, file handoffs and homes.
- `src/features/tasks/`: task submission/status/cancellation/results.
- `src/features/scheduling/`: schedule and run modules. The current router has no schedule page.
- `src/features/history/`: private transcripts, activity and safe operational logs.
- `src/features/connection-profiles/`: global profiles and pure connection-policy parsing/presentation.
- `src/features/project-assistant/`: Hermes and Matrix configuration. Assistant startup is one backend operation.
- `src/features/projects/`, `platform/`: feature-specific clients and views.
- `src/shared/`: HTTP/error handling, project query-key conventions, common transport types and presentation helpers.
- `src/components/ui/`: reusable UI primitives. `src/styles.css` owns theme/layout recipes; `DESIGN.md` records the visual direction.

Feature data and query definitions live beside their feature, in `api.ts` (or history's named API modules). Import another feature's data contract rather than its screen to obtain a query hook. The workspace view intentionally composes the public task and history panels. Shared production modules must not import features or application code.

Every coding workspace has the same fixed confined file and Git MCP tooling; there is no tool-profile catalog or selector. Connection profiles control networking independently.

The backend remains authoritative for project authorization, connection-policy enforcement, lifecycle coordination and scheduling. Frontend transition helpers supply presentation only. Keep project IDs in query keys and verify response ownership before displaying scoped data. Avoid placing workflow orchestration in forms.

Provider and management credentials stay in the trusted backend ingress. Connection tickets remain in URL fragments. Private transcript/result queries use a zero inactive-cache lifetime; closing their last view removes content from the query cache. Task dialogs also remove task queries on close, while accepted tasks continue running. Nothing persists private query data to browser storage.

`npm test` runs source unit tests only. Browser contracts live under `tests/` and run through Playwright, so changing one runner cannot silently skip the other. Typechecking rejects unused locals and parameters. `npm run format` formats frontend sources and test/config files.

The Workers page uses the sidebar as the sole project selector; switching projects
preserves the selected section. Worker rows show configured allocations first,
with compact actions and project-level logs/history/file handoff in Project actions.
Check & start opens a read-only startup review, including actionable controller,
resource, policy, and quota blockers. Only a passed check enables Start worker;
failed checks never start compute. The start request retains its idempotency key
on retry and the server rechecks admission. `tests/worker-readiness.spec.ts` covers
blocked and unavailable checks, recovery, failed starts, cancellation, and mobile.

### Worker debugging terminal

On the Workers page, **Open terminal** opens an interactive shell in a ready
worker's workspace directory. It supports command input, Ctrl+C, tab completion,
Escape, and terminal resizing. Escape is sent to the shell; use **Close terminal**
to close the dialog. Closing ends the shell and its job-control session. If the
connection disappears, the session expires after 60 seconds without a heartbeat.
There are at most four sessions per worker, with 256 KiB of buffered output each.

Terminal access is restricted to the local operator; project/agent tokens cannot
open or interact with debugging shells. Opening a terminal never starts a stopped
worker. Terminal data is transient and does not become a task or conversation.

This requires updated dashboard, control-plane, runtime-controller, and workspace
binaries. Rebuild/redeploy runtime images and restart existing workers to load the
new workspace service. Old workers report terminal unavailable; they are not
silently restarted. The browser uses authenticated, bounded POST exchanges over
the existing runtime transport, so no extra terminal port is exposed.
