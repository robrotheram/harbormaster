+++
title = "Guest execution and artifacts"
aliases = ["/docs/workspace-factory-contract/"]
+++

The workspace bearer token protects these endpoints. The control plane checks project ownership before forwarding. Operations run inside the selected worker and accept no host targets or runtime overrides.

## How a work attempt runs

![The control plane records an attempt and dispatches its specification reference and acceptance key. The worker verifies the digest, commits a receipt, and runs the agent. After a successful task, the worker snapshots declared artifacts and the control plane records their references for review.](diagrams/work-attempt.svg)

The work orchestrator dispatches through the runtime controller with `work:<attempt ID>` as the acceptance key. The worker reads the specification from private storage on first acceptance. On a retry, the receipt identifies the original task. Once the task succeeds, the orchestrator asks the worker to publish each declared artifact before marking the attempt `awaiting_review`.

## Submit a task from a file

`POST /v1/tasks/from-file` accepts `instruction_path`, `idempotency_key`, optional `provider`, and optional `specification_digest`. The digest is a SHA-256 value with 64 lowercase hexadecimal characters. The worker hashes the actual instruction bytes at first acceptance. A mismatch returns HTTP 409 `SPECIFICATION_DIGEST_MISMATCH`.

A retry checks the durable receipt before reading the file. Changing the path, provider, or digest for an existing key returns HTTP 409 `IDEMPOTENCY_CONFLICT`. An existing receipt created without a digest continues to accept retries without a digest.

## Publish and read artifacts

| Request | Contract |
| --- | --- |
| `POST /v1/artifacts` | Accepts `source_path` and `idempotency_key`. Copies a confined regular file up to 32 MiB into private SQLite. Returns HTTP 201 with `id`, `source_path`, `sha256`, `size_bytes`, and `created_at`. |
| `GET /v1/artifacts` | Returns `{"items": [...]}`. |
| `GET /v1/artifacts/{id}/content` | Returns the immutable bytes as `application/octet-stream`. |

A retry with the original path returns the stored artifact and HTTP 200 even if the source file changed or disappeared. A different path with the same key returns HTTP 409. The APIs omit host paths. Artifacts persist independently of source files and running compute.
