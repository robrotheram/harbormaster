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
```

On a fresh Linux CI image, install browser system dependencies with `npx playwright install --with-deps chromium`. The suite starts and stops its own Vite host and a synthetic ingress on loopback ports 4173 and 4174. It covers protected proxy mutations and rejected foreign origins, project switching, workspace creation, pairing, handoffs, scheduling, task cancellation and escaped results, settings, theme/focus/mobile behavior, and the single backend assistant-start operation. Application fixtures are synthetic; this does not replace live runtime or production ingress acceptance tests. Browser artifacts are written to `artifacts/`.

## Ownership and dependencies

- `src/app/`: bootstrap, routing, providers and application shell. Feature screens load lazily.
- `src/features/workspaces/`: inventory, workspace creation/settings, native attachment, file handoffs and homes.
- `src/features/tasks/`: task submission/status/cancellation/results.
- `src/features/scheduling/`: schedules and scheduled runs.
- `src/features/history/`: private transcripts, activity and safe operational logs.
- `src/features/connection-profiles/`: global profiles and pure connection-policy parsing/presentation.
- `src/features/project-assistant/`: Hermes and Matrix configuration. Assistant startup is one backend operation.
- `src/features/projects/`, `tool-profiles/`, `platform/`: feature-specific clients and views.
- `src/shared/`: HTTP/error handling, project query-key conventions, common transport types and presentation helpers.
- `src/components/ui/`: reusable UI primitives. `src/styles.css` owns theme/layout recipes; `DESIGN.md` records the visual direction.

Feature data and query definitions live beside their feature, in `api.ts` (or history's named API modules). Import another feature's data contract rather than its screen to obtain a query hook. The workspace view intentionally composes the public task and history panels. Shared production modules must not import features or application code.

The backend remains authoritative for project authorization, connection-policy enforcement, lifecycle coordination and scheduling. Frontend transition helpers supply presentation only. Keep project IDs in query keys and verify response ownership before displaying scoped data. Avoid placing workflow orchestration in forms.

Provider and management credentials stay in the trusted backend ingress. Connection tickets remain in URL fragments. Private transcript/result queries use a zero inactive-cache lifetime; closing their last view removes content from the query cache. Task dialogs also remove task queries on close, while accepted tasks continue running. Nothing persists private query data to browser storage.

`npm test` runs source unit tests only. Browser contracts live under `tests/` and run through Playwright, so changing one runner cannot silently skip the other. Typechecking rejects unused locals and parameters. `npm run format` formats frontend sources and test/config files.
