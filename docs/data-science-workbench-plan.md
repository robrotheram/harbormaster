# Data science workbench integration plan

## Repository findings

| Area | Extension point | Required change |
| --- | --- | --- |
| Authenticated gateway | `internal/dashboard/interactive.go`, `internal/shared/management_routes.go`, and `internal/api/server.go` | Register explicit routes under `/ui-api/workspaces/{id}/analysis` and `/v1/workspaces/{id}/analysis`. Check project authorization before contacting the worker. Deny hosted human access on shared workers. |
| Runtime transport | `internal/runtime/protocol/controller.go`, `protocol.go`, `internal/runtime/controllerclient/client.go`, and the Docker, Kubernetes, and Firecracker adapters | Use the existing agent route allowlist and workspace grant. Carry the upload as a capped reader across each hop. Keep query requests and results bounded on the existing JSON path. |
| Worker storage and handoff | `internal/workspace/manager.go`, `home.go`, `files.go`, and `review_files.go` | Use a worker-owned directory beside the `files` checkout. Reuse the handoff's confined file access and atomic publication patterns. Handoff itself buffers base64 JSON and caps content at 1 MiB. |
| Existing artifacts | `internal/workspace/artifacts.go` and `internal/api/work_execution.go` | Retain small Markdown reports and specification files through the existing output paths. Store dataset bytes and query results on the worker filesystem. Existing published artifacts store bytes in SQLite. |
| Agent execution | `internal/workspace/native.go`, `internal/workspace/nativeagent/prepare.go`, and `Dockerfile` | Provide a local helper and a pinned native DuckDB Python package. Add data-reference guidance when the worker starts a native agent. The stored task instructions keep their existing byte limits and digests. Use the agent shell for SQL and Python. |
| Visualization | `frontend/src/features/work/chart-engine.ts`, `message-content.tsx`, `reply-outputs.ts`, and `work-output.tsx` | Reuse the ECharts engine. Add table and lazy geo renderers. Share a fenced reference between replies and reports. The existing inline chart format stays supported. |
| Worker and report UI | `frontend/src/features/workspaces/worker-page.tsx`, `frontend/src/features/delivery/review-viewer.tsx`, and the task file viewer | Put ingestion and schema inspection on the worker page. Resolve report references against the attempt's worker and show an unavailable state if worker data is gone. |
| Git | `internal/workspace/task_changes.go` and `docs/interactive-workspaces.md` | Keep reproducibility files in the existing checkout and use its normal review and Git workflow. Agents can publish versioned specs, dependency manifests, queries, code, and reports using standard Git. |

## Review sequence

1. Add worker-local ingestion and transport. Stream a single CSV or Parquet file into staging, calculate its SHA-256, inspect its schema, and atomically publish metadata and data under a generated ID. Reject invalid formats and enforce upload and worker storage limits.
2. Add bounded previews. Use a native DuckDB subprocess with a deadline, per-worker concurrency control, and memory and disk budgets. Generate queries from typed projection, filter, and sort parameters. Use source position as a stable tie breaker. Bind signed cursors to the source and query. Display sample and truncation metadata. Execute arbitrary analytical SQL only in the existing agent shell.
3. Validate the version 1.0 artifact union on the worker. Check source ownership, encoding types, and shape before accepting a specification. Use immutable registrations so retries and later edits cannot change existing report references.
4. Add the analysis UI to the worker and task views. Use existing Workbench colors, type, and controls. Align table headers and cells for scanning. Put status and bounds near the data. Load deck.gl and MapLibre only for geo artifacts. Use a local basemap for geographic data and an orthographic view for projected and scientific coordinates.
5. Add agent instructions and Markdown reporting. Document the local helper, dataset dependencies, and Git publication. Retain report text and specifications through existing output paths. Keep the worker ID implicit in the report's attempt context.
6. Verify the four demonstration scenarios. Exercise large-file uploads and paginated reads, chart and map references, the unavailable state after deletion, and authorization before worker dispatch. Include tests for cancellation, query limits, malformed contracts, and worker isolation.

Projected coordinates use planar rendering in the declared CRS. An agent can transform them to EPSG:4326 before creating an artifact that uses a basemap. DuckDB's `memory_limit` covers its buffer manager, so bound the query subprocess separately. [DuckDB limits](https://duckdb.org/docs/current/operations_manual/limits)
