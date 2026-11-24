+++
title = "Code map and dependency rules"
+++

Use this map to find the package that owns a change. See the [component views](../architecture/components.md) for runtime relationships.

## Backend ownership

| Package | Responsibility |
| --- | --- |
| `internal/api` | HTTP decoding, project authorization, and application operation composition |
| `internal/dashboard` | Browser sign-in, explicit ingress routes, worker attachment, and frontend assets |
| `internal/mcpserver` | Authenticated MCP transport backed by a bounded control-plane client |
| `internal/lifecycle` | Workspace state projection, idle-stop rules, and network transition rules |
| `internal/workspacesvc` | Startup, readiness, quota reservation, and applied workspace configuration |
| `internal/orchestration` | Work attempts, scheduled occurrences, dispatch, and recovery. Its repository owns scheduling SQL. |
| `internal/store` | Platform metadata, ownership checks, profile revisions, and startup migrations |
| `internal/runtime/protocol` | Runtime requests, capabilities, generation grants, and workspace route policy |
| `internal/runtime/controllerclient` | Client for authenticated runtime-controller operations |
| `internal/runtime/{docker,kubernetes,firecracker}` | Runtime lifecycle and provider-specific isolation controls |
| `internal/workspace` | Worker supervisor, task and file endpoints, and history and activity collection |
| `internal/workspace/filetools` | Filesystem operations confined to an owned `os.Root` |
| `internal/workspace/taskstore` | Private SQLite task schema, durable acceptance, and recovery |
| `internal/workspace/nativeagent` | Codex, Claude, and Hermes setup, processes, and result protocols |
| `internal/{gateway,broker,egress}` | Policy enforcement for model requests, named data operations, and outbound destinations |

Runtime providers share the neutral protocol and operate independently. Application packages depend on that protocol and the controller client. [Go architecture tests](../../../../internal/architecture/dependencies_test.go) enforce the import rules.

## Independent frontend features

`frontend/src/app` owns bootstrap, routing, and the shell. It loads feature screens on demand. Each feature keeps its API client and query keys beside its views. To share a query, import the feature's API module. Shared components and HTTP utilities have no feature dependencies.

The browser owns navigation, presentation, and local drafts. The backend owns multi-step operations such as assistant selection and startup. If multiple workers match the project assistant request, the backend returns a conflict and lets the caller choose a worker. See the [frontend development guide](frontend.md) for browser contracts and tests.
