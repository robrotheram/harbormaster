+++
title = "Assistant handoffs"
+++

All content is explicitly selected and private. Exchange exports live in the
source project's immutable blob store; central SQL holds references, digests,
provenance and work receipts only. No assistant receives another project's token,
memory, vault or filesystem. Human grant administration is absent from MCP.

## Setup

Start the control plane and create the source and destination projects. Configure a Hermes worker in each project. Use the project IDs returned by the API. Follow [Hermes installation](operations/hermes.md) for the guest image and host broker. The current image includes `hermes-consume`.

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

Copy `workspace/hermes/examples/consumer-task.md` to `jobs/consumer-task.md` in the
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

See the [exchange transport contract](architecture/assistant-exchanges.md) for paging, authorization, and retry behavior.
