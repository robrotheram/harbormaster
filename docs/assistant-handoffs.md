# Assistant handoffs

All content is explicitly selected and private. Exchange exports live in the
source project's immutable blob store; central SQL holds references, digests,
provenance and work receipts only. No assistant receives another project's token,
memory, vault or filesystem. Human grant administration is absent from MCP.

## Setup

Start the current control plane (startup owns migrations 1–6). Configure the
Personal and destination projects and a Hermes workspace in each, using the
project IDs returned by the API. Follow deploy/hermes/README.md for the pinned
Firecracker image, generation broker and project coordination configuration.
Rebuild the guest image to install /usr/local/bin/hermes-consume.

With an administrator token, create and independently approve a request grant:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  "$WORKBENCH_URL/v1/project-exchanges" -d \
  '{"id":"personal-request","source":"PERSONAL_ID","destination":"TARGET_ID","operation":"work-request"}'
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  "$WORKBENCH_URL/v1/project-exchanges/personal-request/approve" -d '{"project_id":"TARGET_ID"}'
```

Replace uppercase IDs first. Human administration authorizes both projects;
project credentials cannot create or approve grants. Either endpoint can be
revoked by the human admin using POST /v1/project-exchanges/{id}/revoke and
{"project_id":"ENDPOINT_ID"}. Revocation is terminal.

Place a reviewed private specification in the destination workspace. Publish only
its destination workspace_id, specification_path, specification_digest, kind and
optional artifact_paths as a selected work-request JSON body. This reference is
not permission to read source project files, and normal factory workspace
authorization still applies at the destination.

Personal can publish through exchange_publish under personal-request with a
stable occurrence. Alternatively put that selected JSON object in the digest
source fixture's request field, then run:

```sh
/usr/local/bin/hermes-digest --source jobs/digest-source.json \
  --occurrence selected-day --request-grant personal-request
```

The digest publishes an outward request; it never consumes an incoming request.

Copy `integrations/hermes/examples/consumer-task.md` to `jobs/consumer-task.md` in the
destination Hermes workspace. Replace project/workspace IDs and run_at in
consumer-schedule.json, and POST it to /v1/schedules with the destination
credential. This uses the existing Workbench schedule owner; no Hermes cron.
For a direct fixture run inside the provisioned guest:

```sh
/usr/local/bin/hermes-consume
```

The command prints selected_status (export/work/attempt IDs and state) and any
failures as task output. Retain that output through normal task history.
A separate explicit summary/result grant is required to export selected status.

## Transport and recovery contract

exchange_inbox(project_id, after?, limit?) maps to GET /v1/exchange-inbox.
limit defaults to 50 and is bounded to 1–100. Follow next until empty, then start
the next scan with an empty cursor. IDs are immutable keyset cursors, not dates;
concurrent publications sorting before a cursor appear on the next full scan.
The inbox includes consumed receipts (work_id) so a crash between submission and
execution can be repaired. Only approved, unrevoked, destination-bound exports
with matching source, operation and actor provenance appear. Bodies require
exchange_read and undergo digest verification and a fresh grant check.

exchange_submit rechecks grant and body, forwards only destination-authorized
work creation, and records a receipt only after a successful factory response.
It returns the same factory work item on repeat. A receipt failure after creation
returns an error; retry reconciles through the immutable exchange-{export ID}
factory key. There is no separate caller-controlled acknowledgement.

The consumer scans every page even if an older request fails. It resubmits granted
requests and invokes work_run with exchange-{export ID} as the stable delivery
key; retries return the same attempt. It does not intentionally rerun completed
work. Historical receipts remain discoverable, so scan cost grows with history.
No automatic retention/deletion policy is implied.

The fixed broker/project MCP exposes ten tools: work_create/list/run/attempts/
cancel, exchange_publish/inbox/read/submit, note_publish. Human work_review stays
excluded. Hermes uses the pinned MCP2 stream yielding exactly (read, write).

## Checks

```sh
go test -race ./internal/coordination ./internal/api ./internal/mcpserver ./internal/store ./internal/runtime/firecracker
make hermes-check
HERMES_SOURCE=/tmp/awb-hermes-pinned go test ./internal/api ./internal/runtime/firecracker
PYTHONPATH=integrations/hermes/src:integrations/hermes/tests:/tmp/awb-hermes-pinned /tmp/awb-hermes-pinned/.venv/bin/python -m unittest workbench_hermes_tests.sdk_digest -v
```

Composed SDK/broker tests use synthetic private fixtures, not a booted VM.
PostgreSQL upgrade testing requires TEST_DATABASE_URL; live scheduled VM execution
requires separately provisioned Firecracker infrastructure.
