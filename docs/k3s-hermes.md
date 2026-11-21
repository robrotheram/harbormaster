# Kubernetes Hermes v0.23

Hermes is opt-in. The controller requires `KUBERNETES_HERMES_V023_APPROVED_IMAGE`
to equal `WORKSPACE_IMAGE` exactly (registry digest, no tags), plus `HERMES_MODEL`.
That operator approval asserts compatibility with deploy/hermes/runtime.json and
its source's frozen uv.lock. It is not VM attestation. With
`REQUIRE_VERIFIED_ISOLATION=true`, launch and grant verification remain closed.
Use `false` only in an explicitly approved unverified acceptance installation.
Matrix requests are rejected: Kubernetes has no room/token broker.
Codex/T3 continue using the same workspace image and policies.

GitHub Actions publishes the Hermes-enabled workspace alongside the standard
workspace. To build it locally from the same source revision as your platform,
run from the repository root:

```sh
export HERMES_IMAGE='registry.example/workspace-hermes:acceptance'
docker build --target workspace-hermes \
  --build-arg HERMES_COMMIT="$(jq -r .commit deploy/hermes/runtime.json)" \
  --build-arg HERMES_SOURCE_SHA256="$(jq -r .source_sha256 deploy/hermes/runtime.json)" \
  -t "$HERMES_IMAGE" .
docker push "$HERMES_IMAGE"
docker inspect --format '{{index .RepoDigests 0}}' "$HERMES_IMAGE"
```

Review the resulting digest and test on the actual nodes before approval. The
build imports AIAgent, MatrixAdapter and OlmMachine; workspace readiness repeats
that import using `hermes-runner --version` and checks the pinned version/commit.
An installed script or a Ready Pod alone never establishes Hermes readiness.
No provider fallback is configured.

For a Kustomize installation, add `components/hermes` to the consuming overlay's
`components` list (relative to that overlay). It supplies the private Deployment
and Service without enabling Hermes workspace execution. For an overlay at `deploy/kubernetes/overlays/example`, use this path.

```yaml
components:
  - ../../components/hermes
```

Provision `hermes-mcp-secrets` in `harbormaster` outside Git with
`project_signing_key`, `mcp_token` and `gateway_service_token`. Pin the MCP image and
set the project label in your overlay. It has no service
account token and reaches the controller verifier through its trusted service
credential. Do not give those credentials to a workspace.

After native integration acceptance, add a strategic-merge Deployment patch for
`runtime-controller` setting `WORKSPACE_IMAGE`,
`KUBERNETES_HERMES_V023_APPROVED_IMAGE` to the **same reviewed immutable digest**,
and `HERMES_MODEL` to the operator-selected model. Where workspace DNS is
blocked, pin the private Hermes MCP Service's ClusterIP in the overlay and set
`KUBERNETES_HERMES_MCP_IP` to that address. Hermes workspace Pods map only the
fixed MCP hostname to this address and continue to use the existing gateway
network policy. Keep the verified-isolation gate
unless explicitly using an approved unverified test environment. Changing the
workspace image also requires a plan for existing retained Sandboxes.

Build and review with `kubectl kustomize`, then apply the complete consuming
overlay with `kubectl apply -k`. Do not use imperative `set env` mutations that
future Argo reconciliation would overwrite.

The launch contract is `HERMES_MCP_TRANSPORT=kubernetes-generation`, mapped by
Go's typed transport to the single fixed URL
`http://hermes-mcp.harbormaster.svc:8080/hermes-mcp`. Unknown transport values
fail. Firecracker's default remains loopback port 18085 with no NIC requirement.
The guest receives only its workspace generation grant. Every MCP request calls
the trusted controller `/v1/grants/verify` with audience `hermes-mcp`; the live,
owned, Ready, approved-image Hermes sandbox supplies project identity. Caller
project headers have no authority. Suspended, old-generation and foreign grants
fail. Signing happens inside MCP; no global key is sent to a guest.

The endpoint accepts JSON-RPC initialization, ping, discovery, and calls from the [shared project tool list](../internal/shared/project-tools.json). Human review and administration use separate routes. The canonical
network policies are unchanged: blocked still allows only platform gateways;
allowlist internet traffic still requires the authenticated egress gateway.
No guest-chosen upstream, host mount, privileged flag, or SA token is introduced.

Run `go test ./internal/runtime/kubernetes ./internal/mcpserver
./internal/workspace/... ./cmd/workspace` and the Hermes package tests (`make hermes-check`).
Actual image acceptance, CNI deny/allow probes, cross-project isolation, restart
revocation, Kata VM evidence, and production approval must be performed on k3s.
RuntimeClass and NetworkPolicy API objects do not prove VM/network isolation.
This change deliberately does not turn those external gates green.
