+++
title = "Develop Harbormaster"
linkTitle = "Development"
+++

Start here when you need to change Harbormaster. Use the code map to find the owner of a feature, then choose the smallest check that exercises the change. The root [AGENTS.md](../../../../AGENTS.md) gives coding agents the same starting context.

## Set up the repository

Install the Go version in [go.mod](../../../../go.mod), Node.js 24, npm, Python 3.12 or later, and Make. Install Docker Engine and Compose to run the platform or its smoke test. The source checks fetch dependencies and run without provider accounts.

```sh
git clone https://github.com/robrotheram/harbormaster.git
cd harbormaster
go mod download
npm --prefix frontend ci
```

## Find the code you need

| Change | Start here |
| --- | --- |
| Project permissions, worker startup, or task dispatch | [Backend ownership](code-map.md#backend-ownership) and [request flows](../architecture/request-flows.md) |
| Dashboard interactions | [Frontend development](frontend.md) and the feature folder in `frontend/src/features/` |
| Worker previews or agent-generated outputs | [Preview and output behaviour](preview.md) |
| Datasets, charts, or reports | [Extend data analysis](analysis.md) and the [worker analysis API](../architecture/analysis-api.md) |
| Hermes integration | [Hermes package](hermes.md) |
| Documentation or diagrams | [Build and serve the docs](#build-and-serve-the-docs) |

## Run the frontend against a local backend

1. Start the [local Compose deployment](../deployment.md#start-the-local-compose-deployment).
2. From the repository root, run `npm --prefix frontend run dev`.
3. Open `http://127.0.0.1:5173/app/` or use the port Vite reports. Sign in with the local dashboard credentials if prompted.

Set `WORKBENCH_PROXY_TARGET` when the dashboard uses another address. Use the built dashboard origin to test hosted authentication, connection pages, and isolated previews. The [frontend guide](frontend.md) lists the test hosts and suites.

## Build a workspace image

From the repository root, build the standard worker image.

```sh
docker build -f Dockerfile --target workspace -t harbormaster-workspace:local .
```

The [workspace Dockerfile](../../../../Dockerfile) uses the repository root as its build context for shared Go code. The root Dockerfile builds both the platform and worker images. The `tools/` directory holds pinned worker dependencies and the viewer patch. The Hermes adapter is in `integrations/hermes/`. See the [Hermes image build](../k3s-hermes.md) and [Firecracker guest build](../firecracker.md#installation) for their additional build arguments.

## Check your changes

Run checks that exercise the changed behavior, then run `make check` from the repository root.

| Check | Command |
| --- | --- |
| Go formatting, build, race tests, vet, plus frontend, Hermes, and Kubernetes checks | `make check` |
| Workspace HTTP and lifecycle tests | `go test -race ./internal/api ./internal/workspacesvc` |
| Frontend unit tests | `npm --prefix frontend test` |
| TypeScript checks | `npm --prefix frontend run typecheck` |
| Hermes adapter tests | `make hermes-check` |
| Kubernetes manifest and deployment contracts | `make kubernetes-check` |
| Compose configuration | `docker compose config --quiet` |
| Go dependency review | `make go-vuln-check` |

Format changed Go files with `gofmt`. For frontend changes, use `npm --prefix frontend run format`. Review the diff before committing.

### Run browser tests

Run `npm --prefix frontend run test:browser:install` to install Chromium. Run `make browser-check` from the repository root. See [browser suites](frontend.md#browser-suites) for system dependencies, individual specs, and the dashboard connection-page suite.

### Check PostgreSQL and the running platform

1. For a metadata change, set `TEST_DATABASE_URL` to disposable PostgreSQL 17. Use a role that can create schemas. Run `make postgres-check`.
2. For image, container lifecycle, or fixture execution changes, run `make up` and then `make smoke` on a local development installation.
3. For provider, storage, ingress, or isolation changes, run the relevant [deployment acceptance procedure](../deployment.md) on the intended infrastructure.

## Build and serve the docs

Install [Hugo](https://gohugo.io/installation/) v0.147.9 or later. Run `make docs` from the repository root and open `http://localhost:1313/`. Hugo reloads the site when you edit a page.

Edit Markdown in `docs/content/docs/`. Use a section's `_index.md` for its overview and `docs/data/navigation.toml` for sidebar entries. Use relative Markdown links to pages and repository files so they work in GitHub source views. Hugo resolves page links to site URLs and links to other repository files to GitHub. If a published page moves, add its old URL to `aliases` in the new page's front matter.

Run `make docs-build` to regenerate `public/` and remove old output. To test a site hosted under a URL prefix, set `HUGO_BASEURL` to the full URL with a trailing slash. The [Pages workflow](../../../../.github/workflows/docs-pages.yml) publishes changes to the [documentation site](https://robrotheram.github.io/harbormaster/) from `main`.

### Update an architecture diagram

Find the source in [the architecture diagrams directory](../architecture/diagrams). The C4 views and selected request flows use `.drawio` files. Open the file in [diagrams.net](https://app.diagrams.net/) or [draw.io Desktop](https://github.com/jgraph/drawio-desktop/releases). Export it as SVG using the diagram’s original colours. With draw.io Desktop, you can export all diagrams.net sources from the repository root.

```sh
diagram_dir=docs/content/docs/architecture/diagrams
drawio --export --format svg --theme light --embed-svg-fonts false \
	--border 16 --output "$diagram_dir" "$diagram_dir/"*.drawio
```

For a flow with a `.mmd` source, use Mermaid CLI.

```sh
for diagram in docs/content/docs/architecture/diagrams/*.mmd; do
	npx --yes @mermaid-js/mermaid-cli@11.12.0 \
		-i "$diagram" -o "${diagram%.mmd}.svg" -c docs/mermaid.json -b '#171717'
done
```

Keep the source and its SVG together. Update the image's alternative text if the flow changes. Hugo serves the checked-in SVGs.

### Capture a dashboard screenshot

Use a local dashboard with a sample project or the synthetic fixtures in [dashboard browser tests](../../../../frontend/tests). Use dark mode and capture the controls needed for the step. Save the PNG in `docs/static/screenshots/` and describe the controls in the image’s alternative text. Keep real project data and credentials out of captures.

## Prepare a change for review

Follow the [contribution guide](contributing.md) for compatibility, test evidence, and pull request details.
