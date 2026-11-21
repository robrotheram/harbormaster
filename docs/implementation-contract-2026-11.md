> Historical implementation plan, November 2026. This document records the original integration work, not current module ownership or supported behavior. See [current architecture](architecture.md) and [service contracts](contracts.md).

# Project orchestration implementation contract

> Historical record. See the [documentation index](README.md) for current setup and workflows.

Integration contract for the implementation beginning 1 November 2026. The architecture direction remains the source of requirements. Existing workspace content stays in a labelled `default` legacy project; do not infer ownership from display names or move/delete volumes.

## Shared types and routes

- `Project`: `id`, `name`, `created_at`, `legacy` (optional bool). Local human operator may see multiple projects; a project credential authorizes one immutable project only.
- Workspace retains existing fields and gains `home_id`, `agent_kind` (`t3`, `codex`, `hermes`), `template_id` (optional). `project_id` is required for new UI-created workspaces. Existing profile IDs remain compatible presets. Parent workspace must have the same project. Persistent agent state is private to one workspace by default.
- Worker private state persists in runtime storage, identified by the workspace’s immutable `home_id`. The separate AgentHome catalog is retired; historical rows are retained without active readers or writers.
- `GET /v1/projects`, `POST /v1/projects {name,idempotency_key}`. Response list `{items:[]}` or Project.
- `GET /v1/workspaces?project_id=...`, `POST /v1/workspaces` accepts project/home/agent selection as above; backend generates home IDs. The former `GET /v1/agent-homes` endpoint and `agent_home_list` MCP tool are retired.
- Existing `/v1/profiles`, lifecycle, task, console, desktop, usage and file-handoff routes remain. Every operation is authorized by verified request context; project credentials cannot list, execute, read, attach, seed or hand off across projects. Same-project handoffs remain explicit. Cross-project transfers denied even to the human operator until a dedicated export policy exists.
- `/ui-api` is the existing local-human ingress; frontend uses it and never embeds management credentials. Parent extends its exact route allowlist to the above and schedule/run routes.

## Runtime integration

Extend `runtime.WorkspaceSpec` with `ProjectID`, `HomeID`, `AgentKind`, `TemplateID`. Backend passes these from authoritative workspace metadata on start. Runtime labels containers/volumes with project/home and rejects ownership changes. Legacy missing ownership may be adopted only as `default` legacy, never another project. Native T3 browser and desktop paths retain native auth behavior.

Kubernetes backend should reuse pinned Agent Sandbox resources and enforce a fixed administrator template/RuntimeClass. Do not invent verified VM/network evidence. External cluster, Matrix and Claude credentials are not available by implication. No paid API fallback. Keep Docker development working.

## Parent-owned scheduling and integration

Parent implements schedule/run metadata and API in `internal/orchestration`, `internal/api/orchestration.go`, and shared schedule DTOs in `internal/shared/orchestration.go`. Backend agent inserts `s.registerOrchestration(secured)` and invokes `s.tickOrchestration(ctx)` from reconciliation loop when parent supplies it. Other agents do not edit those parent-owned files.

- `GET /v1/schedules?project_id=...` -> `{items:[]}`.
- `POST /v1/schedules {project_id,workspace_id,name,instruction_path,provider,run_at,interval_seconds,idempotency_key}`. Relative project file holds instructions; platform stores only its path/ref, never instruction contents. `provider=codex` initially. `run_at` RFC3339; interval 0 is one-off, otherwise >=60 seconds.
- `POST /v1/schedules/{id}/enable|disable|run-now {idempotency_key}`.
- Schedule fields: `id,project_id,workspace_id,name,instruction_path,provider,enabled,run_at,interval_seconds,next_run_at,last_run_at,last_error_code,created_at`.
- `GET /v1/runs?project_id=...` -> `{items:[]}`. Run: `id,project_id,workspace_id,schedule_id,task_id,state,created_at,updated_at,error_code`. Durable occurrence identity; submission idempotency retained in workspace DB. No replay of interrupted side effects.
- Parent owns MCP transport updates, project token provisioning, dashboard Go ingress/build integration, Compose integration and documentation. Agent UI owns `frontend/` only. Runtime agent owns `internal/runtime/`, `internal/workspace/`, `cmd/runtime-controller/`, `cmd/workspace/`, `deploy/` only. Backend agent owns shared project/model types, `internal/store/`, `internal/api/` except parent file, and `cmd/controlplane/`.
