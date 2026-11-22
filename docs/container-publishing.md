# Container publishing

[GitHub Actions](../.github/workflows/check.yml) runs source and deployment
checks on pushes to `main`, version tags, pull requests and manual dispatch.
Documentation, deployment pin updates and other changes outside the image inputs
skip container work. A version tag reuses the scanned image digests from a
successful `main` push for the same commit if all nine artifacts are available.
Otherwise, the tag runs the full checks and image matrix. Manual runs check the
full image matrix. Pull requests and manual runs on feature branches build and
scan on the CI runner. Registry publishing is limited to `main` and version tags.

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
- `workspace-hermes`

Images target Linux AMD64. The Firecracker guest filesystem is built separately.
Registry builds include provenance and SBOM attestations. The matrix scans each
image with Trivy and reports high and critical findings in its Actions log.
Review findings before deploying a release. The workflow fails when a scan
cannot run successfully.

Each scanned registry build has a `sha-<full-40-character-commit>` tag. When
the image matrix runs, it reuses existing commit images and scans them.
An eligible successful `main` run advances `edge` after the full image set
is available. Releases publish the exact version tag for all nine images,
along with the compatibility tag `workspace:hermes-vX.Y.Z` for the Hermes image.

Authenticate Docker to GHCR for private packages. The workflow publishes with
the repository's short-lived GitHub token and package write permission.

## Releasing and deploying

Once the desired commit has passed checks, push a version tag such as
`v0.3.0`. Image digest artifacts are retained for seven days. The workflow
publishes a GitHub Release with these assets:

- `images.json` records the source revision and immutable image references.
- `kubernetes-base.yaml` renders the Kubernetes base with release digests.
- `kubernetes-hermes.yaml` adds the Hermes MCP component and selects the Hermes
  workspace digest.

The rendered manifests still require environment configuration such as storage,
runtime and secrets. Use the release inventory when updating a Kustomize
environment overlay and submit the change for review. Use the release digests
for Compose overrides as well, including the runtime controller's
`WORKSPACE_IMAGE`. Pull the selected workspace image on the runtime host before
allocating workspaces.

See the [release checklist](operations.md) for acceptance and backup steps.
