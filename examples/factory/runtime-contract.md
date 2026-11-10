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
- `POST /v1/recipe-jobs`: template=`research-v1`, idempotency_key,
  resource_class=`cpu-small` (default), timeout_seconds=1..3600 (default 600).
  Returns 202 job metadata: id, template, state, error_code, created_at,
  timeout_seconds. Exact retry returns 200; changed template/budget is 409.
- `GET /v1/recipe-jobs/{id}` returns that metadata.
- `POST /v1/recipe-jobs/{id}/cancel` requests process-group termination.

Research runs the real standard-library seeded regression fixture for baseline
and regularisation=10. Results and execution.log are at logical project paths
`outputs/{job-id}/baseline.json`, `variation.json`, `execution.log`; publish these
explicitly with the artifact API. Publication is separate from job success.
Linux prlimit and python3 are required. Bounds: 1 GiB virtual address space,
3600 CPU seconds, 64 processes, 32 MiB per file, and the requested wall deadline.
This is a bounded process profile, not allocation of a dedicated CPU core.
Jobs participate in idle activity and exclude concurrent native task acceptance.
Shutdown cancels and waits; recovery marks unconfirmed running jobs interrupted
with SUPERVISOR_RESTART and never replays them. The existing home writer lock
continues to own the private database.

## Remaining acceptance gates

GoGallery is not yet an admitted executable template. Unsupported templates fail
422. The example environment.json files remain descriptive, not authority.
Authenticated service forwarding and independent retained-preview allocation are
not implemented by this foundation; no arbitrary target proxy was introduced.
No Firecracker service capability is advertised. M2 is therefore not accepted.
Resource isolation is additional to, and never replaces, sandbox enforcement.
