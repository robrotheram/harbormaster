# Kubernetes Hermes v0.23

Hermes is opt-in. The controller requires `KUBERNETES_HERMES_V023_APPROVED_IMAGE`
to equal `WORKSPACE_IMAGE` exactly (registry digest, no tags), plus `HERMES_MODEL`.
That operator approval asserts compatibility with deploy/hermes/runtime.json and
its source's frozen uv.lock. It is not VM attestation. With
`REQUIRE_VERIFIED_ISOLATION=true`, launch and grant verification remain closed.
Use `false` only in an explicitly approved unverified acceptance installation.
Matrix requests are rejected: Kubernetes has no room/token broker.
Codex/T3 continue using the same workspace image and policies.

Build the optional overlay from repository root, with a reviewed Debian
Bookworm-compatible workspace base containing the current workspace binary:

```sh
export WORKSPACE_BASE='registry.example/workspace@sha256:REPLACE_WITH_REAL_DIGEST'
export HERMES_IMAGE='registry.example/workspace-hermes:acceptance'
docker build -f deploy/kubernetes/Dockerfile.hermes \
  --build-arg WORKSPACE_BASE="$WORKSPACE_BASE" \
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

On an existing installation produced by deploy/kubernetes/render_platform.py:

1. Render deploy/kubernetes/hermes-mcp-deployment.yaml with the bound project
   ID and current MCP image digest, provision hermes-mcp-secrets with the three
   file keys listed there, and apply it with `kubectl apply -f <rendered-file>`.
   Deploy a dedicated MCP instance in workbench-egress named hermes-mcp using
   the current MCP binary and pinned image. Use the existing MCP deployment's
   read-only control/MCP secret file mounts and security context, without a
   service account token. Label its Pod `app=hermes-mcp`,
   `workbench.role=gateway`, and `workbench.project=<bound project>`.
2. Set `MCP_ALLOWED_HOSTS=hermes-mcp.workbench-egress.svc:8080`,
   `HERMES_CONTROLLER_URL=http://runtime-controller.workbench-system.svc:8080`,
   and mount the controller's gateway service credential read-only at
   `GATEWAY_SERVICE_TOKEN_FILE`. Keep CONTROL_TOKEN_FILE and MCP_TOKEN_FILE only
   in this control-plane service. Never place these credentials in a workspace.
   The namespace network policy must permit this service to reach the controller
   verifier and controlplane; retain existing controller ingress restrictions.
3. Apply the Service and set the reviewed runtime environment:

```sh
kubectl apply -f deploy/kubernetes/hermes-mcp-service.yaml
export APPROVED_DIGEST='registry.example/workspace-hermes@sha256:REPLACE_WITH_REAL_DIGEST'
kubectl -n workbench-system set env deployment/runtime-controller \
  WORKSPACE_IMAGE="$APPROVED_DIGEST" \
  KUBERNETES_HERMES_V023_APPROVED_IMAGE="$APPROVED_DIGEST" \
  HERMES_MODEL='operator-selected-model'
```

The launch contract is `HERMES_MCP_TRANSPORT=kubernetes-generation`, mapped by
Go's typed transport to the single fixed URL
`http://hermes-mcp.workbench-egress.svc:8080/hermes-mcp`. Unknown transport values
fail. Firecracker's default remains loopback port 18085 with no NIC requirement.
The guest receives only its workspace generation grant. Every MCP request calls
the trusted controller `/v1/grants/verify` with audience `hermes-mcp`; the live,
owned, Ready, approved-image Hermes sandbox supplies project identity. Caller
project headers have no authority. Suspended, old-generation and foreign grants
fail. Signing happens inside MCP; no global key is sent to a guest.

The endpoint accepts POST JSON-RPC initialization, ping, discovery and only these
calls: work_create/list/run/attempts/cancel, exchange_publish/inbox/read/submit,
note_publish. Human review and administration remain absent. The canonical
network policies are unchanged: blocked still allows only platform gateways;
allowlist internet traffic still requires the authenticated egress gateway.
No guest-chosen upstream, host mount, privileged flag, or SA token is introduced.

Run `go test ./internal/runtime/kubernetes ./internal/mcpserver
./internal/workspace/... ./cmd/workspace` and the scripts/test-hermes-*.py tests.
Actual image acceptance, CNI deny/allow probes, cross-project isolation, restart
revocation, Kata VM evidence, and production approval must be performed on k3s.
RuntimeClass and NetworkPolicy API objects do not prove VM/network isolation.
This change deliberately does not turn those external gates green.
