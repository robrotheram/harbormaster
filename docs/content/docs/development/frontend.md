+++
title = "Develop the frontend"
aliases = ["/docs/reference/frontend/readme/"]
+++

The frontend uses React, TanStack Router and Query, Tailwind CSS, and Radix-backed components. Fonts and icons are bundled locally. Follow the [development guide](./_index.md#run-the-frontend-against-a-local-backend) to run Vite. Vite proxies `/auth`, `/ui-api`, and `/dashboard-api` to the dashboard. The development proxy preserves the browser's Host for ingress Origin validation.

## Browser suites

Run these commands from the repository root after installing the frontend dependencies.

```sh
npm --prefix dashboard run test:browser:install
make browser-check
npm --prefix dashboard run test:connections
```

If Linux needs Chromium system packages, run `npx playwright install --with-deps chromium` from `dashboard/`. To run one spec, use `npx playwright test tests/work.spec.ts` from that directory.

The main suite builds the current frontend. It uses a synthetic ingress on loopback ports 4173 and 4174 and serves the bundle with the dashboard's production content security policy. The connection-page suite needs the Go toolchain. It starts the dashboard with a synthetic control plane on port 5191 and checks connection recovery, one-use tickets, layouts, and accessibility. Both suites use synthetic data and write test output under `dashboard/artifacts/`.

| Workflow | Spec |
| --- | --- |
| Project tasks, assistant entry, infrastructure filters | `tests/software-factory.spec.ts` |
| Bulk worker operations and partial failures | `tests/bulk-workers.spec.ts` |
| Startup readiness, blockers, and recovery | `tests/worker-readiness.spec.ts` |
| Navigation, heading structure, and responsive layout | `tests/ui-consistency.spec.ts` |

Vitest runs the unit tests. See [check your changes](./_index.md#check-your-changes) for type checks and formatting.

## Edit a feature

1. Find the view and API module in `dashboard/src/features/`. See [frontend ownership](code-map.md#independent-frontend-features).
2. Use shared HTTP helpers in `dashboard/src/shared/`. Include the project ID in project query keys and check response ownership.
3. Keep private queries out of browser storage. Private transcript and output queries use a zero inactive cache lifetime. Keep connection tickets in URL fragments.
4. Reuse components in `dashboard/src/components/ui/`. Change shared theme and layout rules in `dashboard/src/styles.css`.

When changing hosted sign-in, check [AuthKit setup](../authkit.md) and the [dashboard contract](../architecture/contracts.md#dashboard). When changing agent threads, check [work conversations](../architecture/contracts.md#work-conversations) and [browser chart presentation](../architecture/contracts.md#browser-chart-presentation).
