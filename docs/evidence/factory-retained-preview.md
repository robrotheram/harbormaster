# Retained gallery implementation contract

Guest API (existing bearer authentication on supervisor port 8080):

- `POST /v1/recipe-jobs`: existing request, now admits `gogallery-v1` in addition to `research-v1`. GoGallery requires the administrator-built `workspace-gogallery` image, otherwise 422 `APPROVED_GOGALLERY_IMAGE_REQUIRED`. No repository manifest is executed as platform configuration. Fixed source: `5f1bfd6a41afec65bc7ab791a14863180d944ee0`. Build/test and gallery generation execute offline from source/module cache baked into the trusted image. Native dependencies are installed during image build.
- Jobs atomically archive execution logs and successful fixed outputs in the existing artifact catalog. GoGallery outputs `gallery.tar`, `provenance.json`; Research outputs `baseline.json`, `variation.json`. Source path contains producer job ID. Failure to archive prevents success. Artifacts survive job and supervisor shutdown.
- `POST /v1/services`: JSON `{snapshot: <base64 uncompressed tar>, ttl_seconds: 1..86400}`. Snapshot maximum 16 MiB; at most 512 regular files, each at most 2 MiB. Reject links, traversal, duplicates, missing index. Optional `site/` prefix stripped. Returns 201 `{id,state,expires_at}`. Immutable file bytes stored in retained environment SQLite, independently of the coding checkout.
- `GET /v1/services/{id}` returns metadata; expired access updates state.
- `POST /v1/services/{id}/fetch`: `{path:"index.html"}` returns JSON `{content:<base64>,content_type,attachment:"isolated-origin-required",websocket:false}`. Maximum response 3 MiB. No direct HTML responses, arbitrary target, process port, redirect or WebSocket tunnel.
- `POST /v1/services/{id}/revoke`: revoke before acknowledgement.
- `POST /v1/services/reconcile`: expire overdue services. Supervisor also reconciles every second. Expired/revoked fetch returns 410; archived bytes remain readable in retained storage. Active services block workspace idle-stop; expired services cease doing so, allowing existing idle reconciliation to release compute.

Neutral protocol explicitly admits only these service paths. Body budget for service creation is 24 MiB, leaving other metadata limits unchanged. Docker and Firecracker existing controller forwarding consume these neutral limits; no new NIC or guest port is required.

Platform integration: `internal/previewsvc.Service.Promote` takes two already-authorized workspace records plus a published artifact ID and TTL. It requires distinct workspace/home IDs, same nonempty project, and running retained allocation; it copies archived bytes through runtime Agent calls. Allocate/persist the dedicated preview workspace through the existing workspace lifecycle/quota operations before calling this method. Never mount or reuse the coding home. The parent owns operator ingress/MCP/UI bindings and allocation policy; the project platform HTTP operations are implemented below. `Attach` deliberately fails with `ISOLATED_PREVIEW_ATTACHMENT_UNSUPPORTED`. Browser rendering must use existing isolated workspace host/session machinery, with revocation checked on each request; never decode this response into operator-origin HTML. No isolated browser attachment is claimed by this implementation.

Build:

```
docker build --build-arg SERVICE=workspace --target workspace-gogallery -t agent-workbench-workspace:gogallery-v1 .
go test -race ./internal/workspace ./internal/previewsvc ./internal/runtime/protocol ./internal/runtime/controllerclient
```

Local acceptance 2026-11-10: GoGallery source HEAD verified against the pinned revision; `go test ./...` and fresh `go build -o /tmp/factory-gallery-test .` succeeded with standard native dependencies. Synthetic fixture `/tmp/factory-gallery-acceptance-11010`, `/tmp/factory-gallery-test --config gallery.yaml build` completed exit 0 (three EXIF images, two albums, three pages). This host executable is verification evidence only and is never packaged as a runtime asset.

Tests use separate real workspace managers/storage, restart the retained manager after coding shutdown, fetch immutable snapshot content, manually revoke, exercise lazy and explicit reconciliation expiry, and verify snapshot evidence remains. Another test crosses controllerclient HTTP serialization with actual guest handlers and artifact publication/promotion. Allocation discovery in that test is a small HTTP wiring adapter, not an actual Docker or Firecracker controller.

Remaining integration acceptance: parent API allocation/auth bindings and isolated host/session attachment; Firecracker image installation and live noNIC forwarding; confirmed compute release by the platform idle controller; production k3s remains unsupported. Guest expiry and revocation do not erase evidence or pretend that failed backend termination released capacity.

`previewsvc.Retain` composes the existing `workspacesvc.Service` lifecycle (Start/Stop interface) with promotion, so normal quotas/configuration remain authoritative. `ReleaseExpired` checks guest terminal service state before invoking Stop; unknown state or failed termination does not free capacity. Parent calls these with authorized persisted records and ordinary workspace coordination locks. Registered allocation creation remains at the parent API boundary.

The image-native build uses a 128 MiB per-file process limit (the standard GoGallery executable is approximately 65 MiB), with artifact publication still separately bounded to 32 MiB. GoGallery is limited to 4 GiB of virtual address space, while the backend's admitted cpu-small physical memory limit remains authoritative.

Live acceptance command (uses disposable, network-disabled Docker guests, never guest Docker credentials):

```
python3 deploy/templates/gogallery/acceptance.py
```

The acceptance probe disables generic `/proc` inventory because its own `docker exec` HTTP client would otherwise count as unrelated guest work. Owned recipe process groups, deadlines, prlimit, and Docker CPU/memory enforcement remain enabled. It exercises actual guest APIs, archives the generated tar, copies it into a different guest, stops coding compute, reads retained index content, revokes it, and reconciles a second preview's expiry. This is Docker guest acceptance, not a claim of live Firecracker enforcement or operator-origin browser attachment.

`deploy/templates/gogallery/template.json` records the administrator image contract; it is descriptive build inventory, not a repository-controlled privilege admission mechanism. Admission remains compiled guest policy plus the trusted installed image. The baseline repository example `environment.json` is not loaded by this service.

Trusted image build completed locally: `sha256:13751157fdbd52d851bd5389e5bad1d74df0f5d3575f88dcb6e4616fb58e1b56`. An independent cold workload invocation exited 0 with `--network none --memory 1g --cpus 1`, `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, and `GOMAXPROCS=1`, running `/opt/workbench/gogallery/run` in disposable writable container scratch. All GoGallery Go tests passed; its build and synthetic gallery tar/provenance generation completed. Logs at `/tmp/factory-image-workload.log` are local transient evidence. No host baseline executable was supplied to this image.

`make check` completed successfully, including full race tests, command builds, frontend checks/build, Python adapter/fixture checks and vet. Subsequent targeted race tests and vet passed after final service changes.

Firecracker's trusted rootfs Dockerfile now accepts administrator build argument `WORKSPACE_IMAGE=agent-workbench-workspace:gogallery-v1` so its native dependencies/toolchain can be baked into the immutable guest rather than fetched or injected by a repository at runtime. Existing root-owned rootfs manifest/hash verification and noNIC/vsock policy remain required. This change does not provision or replace an installed Firecracker rootfs automatically.

Cold-build budget finding: the first authenticated guest run hit its explicitly requested 600-second deadline (`DEADLINE_EXCEEDED`); the supervisor terminated its owned process group. The independent image invocation had already succeeded without that short wall deadline. GoGallery's default is now 1800 seconds (still capped at 3600), while Research retains its 600-second default. The live acceptance script explicitly requests 1800 seconds. Physical CPU/memory and no-network constraints are unchanged.

Authenticated Docker guest acceptance passed in 671 seconds: job `job_c5cbfd73973942b43701eacd3ba797f0` succeeded with its 1800-second budget; all three automatic artifacts were archived. The 276480-byte gallery snapshot remained readable in the retained guest after coding stopped; revocation and reconciliation expiry both returned 410. See `factory-live-acceptance.json` for synthetic IDs, content digests and timestamps. Both disposable guests were stopped and removed after acceptance.

## Platform API integration

New registration is additive: `registerFactoryEnvironments` in `internal/api/server.go`, with no edits to work-attempt, coordination or authentication helpers. Preserve all parent registrations when cherry-picking supervisor/API hunks.

- `POST /v1/workspaces/{id}/recipe-jobs` accepts the guest recipe request through existing project authentication and normal workspace start/quota policy. Unsupported recipes/budgets fail before allocation. Default GoGallery timeout is 1800 seconds. `GET /v1/workspaces/{id}/recipe-jobs/{job}` and `POST .../{job}/cancel` observe/cancel the same durable guest job.
- `POST /v1/workspaces/{coding_id}/retained-previews`: `{retained_workspace_id,artifact_id,ttl_seconds}`. Both records must be authorized and belong to the same project, with distinct workspace/home IDs. The retained workspace must first be registered through existing workspace creation; the operation starts it through normal lifecycle admission and copies immutable artifact bytes. Return is the guest 201 service metadata.
- `GET /v1/workspaces/{retained_id}/services/{service_id}` and POST suffixes `/fetch`, `/revoke`, `/release`. Fetch body is `{path}`; bytes are JSON/base64, with no HTML rendering. Fetch/revoke never restart stopped compute. Release verifies guest expiry/revocation and normal stop policy, returns `{workspace_id,state:"STOPPED"}` only after confirmed termination. Unknown state or failed stop retains capacity.
- Workspace operational metadata gains optional `retained_preview` containing service ID, source workspace/artifact IDs and deadline, stored in the existing versioned JSON metadata without new SQL tables. Normal platform reconciliation checks that deadline independently of idle timeout, verifies guest revocation/expiry, and stops confirmed idle retained compute. Metadata survives platform restart; no source data or HTML enters the control-plane database.

Root UI/MCP can map these `/v1/...` operations through the established `/ui-api` ingress. Work-attempt association remains in the parent's work operations; these routes do not overwrite work projection or task metadata. HTML attachment remains explicitly unsupported until the parent provisions isolated host/session routing.

Cross-layer commands:

```
go test -race ./internal/api -run TestFactory -count=1
CGO_ENABLED=0 go test -c -o /tmp/factory-platform-api.test ./internal/api
docker run --rm --network none --memory 1g --cpus 1 --no-healthcheck --mount type=bind,source=/tmp/factory-platform-api.test,target=/tmp/factory-platform-api.test,readonly --env FACTORY_LIVE_GALLERY=1 --entrypoint /tmp/factory-platform-api.test agent-workbench-workspace:gogallery-v1 -test.run '^TestFactoryProjectAPIExecutesRealRecipeAndFencesTemplates$' -test.timeout=35m -test.v
```

The first command exercises real platform authentication, SQLite metadata, normal lifecycle services, HTTP guest requests, real Research execution and archived outputs; it also promotes into a distinct allocation, stops coding, fetches preview, revokes and confirms release. A separate case restarts the platform server and reconciles retained expiry into confirmed STOPPED capacity. Cross-project requests are denied before allocation. The optional image invocation uses a portable compiled platform test harness (not a host GoGallery binary), admitting and executing the real pinned GoGallery recipe from the platform API. Allocation discovery uses the repository's embedded runtime fixture; workload processes, toolchain, dependencies, filesystem, guest HTTP, SQLite and project API handlers are real. This does not claim a live provider controller or Firecracker VM acceptance.


The live platform API invocation passed in 618.15 seconds (`factory-platform-gallery.txt`): the authenticated project workspace route admitted `gogallery-v1`, observed actual successful build/test/gallery generation and automatic evidence publication, then returned the same job on an idempotent retry. No host GoGallery executable or runtime library was mounted; only the statically compiled platform API test harness was mounted read-only.

Integration baseline: rebased onto parent `69947c9`, preserving coordination API/supervisor registrations, coordination protocol routes and its 100000-byte body limit. No parent work, coordination/auth-helper or Kubernetes files were changed. Integrated `GOFLAGS=-p=1 make check` passed. An earlier parallel run's dashboard vet process was killed; the complete serialized rerun passed without a code change. Targeted factory API/workspace/client/protocol race tests also passed. All acceptance containers have exited and been removed.
