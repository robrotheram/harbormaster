# Guest factory execution foundation

The workspace bearer token protects all endpoints below. Requests never accept
host targets, shell commands, environment overrides, or repository-defined
privileges. These operations run inside the already selected workspace sandbox;
they do not allocate a weaker runtime. Project control-plane forwarding must
retain the existing workspace ownership check.

- `POST /v1/tasks/from-file`: existing instruction_path, provider and
  idempotency_key plus optional specification_digest (64 lowercase hexadecimal
  SHA-256 characters). First acceptance hashes actual file bytes; mismatch is
  409 SPECIFICATION_DIGEST_MISMATCH. Retries resolve the durable receipt before
  reading the file. Changing path/provider/digest, including adding a digest to a
  legacy receipt, returns 409 IDEMPOTENCY_CONFLICT. Empty-digest legacy receipts
  remain valid after the additive private SQLite migration.
- `POST /v1/artifacts`: source_path and idempotency_key. A confined regular file
  up to 32 MiB is copied atomically into private durable SQLite storage. Returns
  201 metadata (id, source_path, sha256, size_bytes, created_at). Retry returns
  200 original metadata even if source changed/disappeared; changed path is 409.
- `GET /v1/artifacts`: `{ "items": [...] }`.
- `GET /v1/artifacts/{id}/content`: immutable bytes, application/octet-stream.
  No host paths are returned. Artifacts have no automatic expiry/deletion and
  remain separate from source files and compute lifetime.
