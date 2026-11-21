# Develop Harbormaster

## Set up the repository

Install the Go version in [go.mod](../go.mod), Node.js 24 as used by CI, npm, Python 3.12 or later, and Make. Install Docker Engine and Compose if you need to run the local platform or smoke test. The source checks fetch dependencies and do not require provider accounts or a live cluster.

```sh
git clone https://github.com/robrotheram/harbormaster.git
cd harbormaster
go mod download
npm --prefix frontend ci
```

## Find the code you need

| Change | Start in |
| --- | --- |
| Project access and HTTP operations | [`internal/api`](../internal/api) and [`internal/shared`](../internal/shared) |
| Worker startup, readiness, and quota | [`internal/workspacesvc`](../internal/workspacesvc), [`internal/lifecycle`](../internal/lifecycle), and [`internal/runtime`](../internal/runtime) |
| Work dispatch and schedules | [`internal/orchestration`](../internal/orchestration) |
| Platform metadata and migrations | [`internal/store`](../internal/store) and the owning repository package |
| Agent processes, private task data, and files | [`internal/workspace`](../internal/workspace) |
| Browser routes, UI API, and connection tickets | [`frontend/src`](../frontend/src) and [`internal/dashboard`](../internal/dashboard) |
| External agent tools | [`internal/mcpserver`](../internal/mcpserver) |
| Hermes Python integration | [`integrations/hermes`](../integrations/hermes) |

Read [architecture and module ownership](architecture.md) before changing an operation that crosses packages. Keep policy in the package that owns the operation. For a frontend workflow, use the feature's API client and keep the project ID in its query keys.

## Run the frontend against a local backend

1. Start the [local Compose deployment](deployment.md#start-the-local-compose-deployment).
2. From the repository root, start Vite.

   ```sh
   npm --prefix frontend run dev
   ```
3. Open `http://127.0.0.1:5173/app/` or use the port Vite reports. If prompted, enter the local dashboard credentials.

Vite proxies `/auth`, `/ui-api`, and `/dashboard-api` to `http://127.0.0.1:8080`. If the dashboard uses another port, set `WORKBENCH_PROXY_TARGET` when you start Vite. Use the built dashboard origin to exercise hosted authentication, worker connection pages, or isolated preview origins. For those flows, the dashboard owns the origin and ticket checks. See the [frontend notes](../frontend/README.md) for test hosts and code ownership.

## Check your changes

Run the checks that exercise the changed behavior, then run `make check` from the repository root.

| Check | Command | What it covers |
| --- | --- | --- |
| Full source checks | `make check` | Go formatting, builds, race tests, and vet. Frontend installation, formatting, tests, and build. Python Hermes tests and Kubernetes contract checks. |
| Focused Go tests | `go test -race ./internal/api ./internal/workspacesvc` | Example for a change to the workspace HTTP and lifecycle boundary. |
| Frontend unit tests | `npm --prefix frontend test` | Source tests with Vitest. |
| Type checking | `npm --prefix frontend run typecheck` | TypeScript types and unused symbols. |
| Hermes adapter | `make hermes-check` | Portable Python tests for adapter behavior. |
| Kubernetes contracts | `make kubernetes-check` | Manifest rendering with pinned Kustomize and deployment tests. |
| Compose validation | `docker compose config --quiet` | Compose configuration and interpolation. |
| Go dependency review | `make go-vuln-check` | Reachable vulnerabilities reported for Go dependencies. |

Use `gofmt` on changed Go files. Use `npm --prefix frontend run format` for the frontend formatting rules. Inspect the resulting diff before committing.

### Run browser tests

Install frontend dependencies first. Install Chromium, then run the Playwright suite.

```sh
npm --prefix frontend run test:browser:install
make browser-check
```

On a fresh Linux environment that needs Chromium system packages, run `npx playwright install --with-deps chromium` from `frontend/`. To run one browser spec, run `npx playwright test tests/work.spec.ts` from the same directory. Use `npm --prefix frontend run test:connections` for the separate suite that starts the Go dashboard against a synthetic control plane.

### Check PostgreSQL and the running platform

1. For a metadata change, provision disposable PostgreSQL 17. Set `TEST_DATABASE_URL` to a database where the test role can create schemas. Run `make postgres-check`. The test uses its own temporary schema. Use a test database.
2. For a change to image contents, container lifecycle, or fixture execution, build and start a local fixture deployment with `make up`. Run `make smoke` against it. Use a development installation you can modify.
3. For a change to provider, storage, ingress, or isolation behavior, run the matching [deployment acceptance procedure](deployment.md) on the intended infrastructure.

## Build and serve the docs

The Hugo project lives in the ignored `website/` directory. Keep a local copy of that directory to build the site.

Install [Hugo Extended](https://gohugo.io/installation/) v0.147.9 or later. Hugo uses Go to fetch the pinned [Lotus Docs theme](https://themes.gohugo.io/themes/lotusdocs/) and its Bootstrap module on the first build. From the repository root, run `make docs-serve`. Open `http://localhost:1313/`. Hugo reloads the site when you edit a Markdown guide.

Run `make docs-build` to generate static files in `public/`. For hosting, set `HUGO_BASEURL` to the full site URL, including any path prefix and a trailing slash, before you build.

Edit the existing files in `docs/`. The site uses `docs/README.md` as the documentation index and mounts the related component guides listed in `website/hugo.toml`. Links to source files outside the site open the GitHub repository.

## Prepare a change for review

Run `git diff --check`. Check that generated artifacts, secrets, private workspace files, and connection traces stay out of the diff. In the pull request, describe the observable change and the checks you ran. For behavior changes at a service boundary, include evidence from a check that crosses that boundary. The [CI workflow](../.github/workflows/check.yml) lists the checks used for pull requests and release builds.
