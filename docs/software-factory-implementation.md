# Software factory implementation status

Updated 10 November 2026. Tracks delivery against
[the plan](software-factory-plan.md).

## Available foundation

Startup migration 3 adds durable `work_items` metadata without changing existing
workspace ownership or task history. Work records reference a private workspace
specification path and SHA-256 digest; specification content remains outside the
shared metadata database. The digest is caller-supplied and is not yet verified
against workspace bytes. Consequently creation always produces a draft and never
starts an agent.

Authenticated operations:

- `POST /v1/work-items` creates a draft using `project_id`, `workspace_id`,
  `specification_path`, `specification_digest`, `kind` (`software`, `research`, or
  `digest`), and `idempotency_key`.
- `GET /v1/work-items?project_id=PROJECT` lists at most 200 records in ID order.
  Operator credentials must supply a project; project credentials infer their
  authorised project when the filter is omitted.
- `GET /v1/work-items/ID?project_id=PROJECT` retrieves a scoped record.
- `POST /v1/work-items/ID/cancel` with `project_id` cancels a draft. Repeating the
  cancellation returns the same cancelled state.

A creation retry with identical references returns the existing record. Changing
its workspace, specification path/digest, or kind with the same key returns 409.
Creation keys are isolated by project. The HTTP operations are also available
through the local operator ingress. MCP exposes `work_create` and `work_list`.

[Example workloads](../examples/factory/README.md) include versioned recipe
proposals and a runnable CPU research fixture. The Python test is part of
`make python-check`. The fixture validates deterministic dataset/model outputs
while retaining elapsed time as an observation.

## Remaining delivery

Local baseline validation passed on 10 November 2026: the pinned GoGallery source
built, its `go test -p 1 ./...` suite passed, and the generated synthetic gallery
built successfully with absolute fixture paths. These runs used the development
host toolchain, not a managed sandbox, and do not establish runtime isolation.
The research baseline, variation, and reproduction test also passed.

M0 remains in progress: complete managed GoGallery preview acceptance,
select artifact storage and service access, and validate runtime capability
contracts. Example JSON recipes are not yet loaded by runtime providers.

M1 has draft metadata and API/MCP access. Specification verification/snapshotting,
execution attempts, schedule integration, artifact publication/retrieval, and the
work-item UI remain to be implemented. There is no work-item execution endpoint.

M2–M6 remain pending, including managed previews, long-running research jobs,
Hermes coordination, Obsidian, and real k3s acceptance. Existing workspace task
submission and scheduling remain the supported execution paths.

## Verification

`GOFLAGS=-p=1 make check` passed, including Go builds, race tests, architecture
checks, vet, frontend formatting/tests/build, the research reproduction test, and
portable Hermes adapter tests. Package concurrency was limited after an earlier
parallel link was killed during concurrent native compilation. PostgreSQL and
live runtime/cluster acceptance were not exercised.
