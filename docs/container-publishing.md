# Container publishing

The [GitHub workflow](../.github/workflows/check.yml) runs on pushes to `main`,
`v*` tags, pull requests, and manual dispatch from the Actions page. The existing
Go/PostgreSQL, frontend/browser, Python and Compose checks must pass before any
container job starts. Pull requests build but never log in or publish. Manual
runs on other branches build only.

## Images and tags

All images use `ghcr.io/robrotheram/harbormaster/<image>`:

- `controlplane`
- `runtime-controller`
- `dashboard`
- `mcp`
- `inference-gateway`
- `egress-gateway`
- `credential-broker`
- `workspace`

Builds currently target **Linux AMD64**. ARM64 support requires acceptance of
bundled native T3/provider binaries and workspace images before adding it to
the matrix. PostgreSQL and Vault remain upstream images. Firecracker guest/rootfs
artifacts and the optional Kubernetes Hermes overlay retain their separate
provisioning workflows; this pipeline builds the root Dockerfile targets only.

Each published build has `sha-<full-40-character-commit>` tags. Successful `main`
builds also update `edge`. Pushing a version tag such as `v1.0.0` publishes that
exact image tag for all eight images. There is deliberately no implicit `latest`.
Re-running a commit can rebuild mutable upstream base tags, so use the resulting
**image digest** for immutable deployments.

For example, after a successful main build:

```sh
docker pull ghcr.io/robrotheram/harbormaster/dashboard:edge
docker pull ghcr.io/robrotheram/harbormaster/workspace:edge
```

Package visibility is controlled by GitHub. If packages are private, authenticate
Docker to GHCR with an account/token permitted to read those packages, or set the
packages' visibility to public in GitHub if anonymous pulls are intended. The
workflow itself uses its short-lived `GITHUB_TOKEN` with `packages: write`; no
Docker Hub password or separate publishing secret is needed.

## Operation and deployment

Jobs use separate BuildKit caches per image, pinned action revisions, SBOM and
provenance attestations. At most three image jobs run concurrently. Each job has
a 45-minute limit. Open its build summary for the published tags, digest and
build diagnostics. A failed image does not cancel the remaining matrix jobs;
require the whole workflow to pass before treating a commit as a complete set.

Publishing does not deploy. The default Compose file continues to build local
development images. For a deployment, supply a reviewed Compose override setting
each service's `image` to its registry digest, plus `WORKSPACE_IMAGE` on the runtime
controller to the published workspace digest, and run with `--no-build`. Pull the
workspace image onto the Docker runtime host before allocating workspaces. The
controller does not gain registry credentials from GitHub Actions. For other
backends, configure image access using that backend's supported mechanism.

Keep runtime-specific acceptance and backups in the [release checklist](operations.md).
