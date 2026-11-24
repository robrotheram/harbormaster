# Working in Harbormaster

Harbormaster runs software and research tasks in project-owned workers. The dashboard and MCP call the Go control plane. The control plane authorizes project operations and calls the runtime controller. Worker code runs agents and keeps their private files and task state.

## Start here

- [Developer guide](docs/content/docs/development/_index.md) covers setup, local commands, and validation.
- [Code map](docs/content/docs/development/code-map.md) identifies package owners and import boundaries.
- [Architecture](docs/content/docs/architecture/_index.md) describes service boundaries, persistence, and supported runtimes.
- [Dashboard guide](docs/content/docs/user-guide.md) shows the user workflow and current UI labels.
- [Service contracts](docs/content/docs/architecture/contracts.md) document selected API routes. Check their implementation and tests for complete behaviour.

## Find the owner of a change

- Project authorization and HTTP composition live in `internal/api`. Lifecycle policy lives in `internal/workspacesvc` and `internal/lifecycle`. Work attempts and schedules live in `internal/orchestration`.
- Runtime contracts live in `internal/runtime/protocol`. Backend implementations stay independent. The Go tests in `internal/architecture` enforce the import boundaries.
- Worker execution and storage live in `internal/workspace`. The worker image and Hermes package live in `workspace/`.
- Dashboard features and browser tests live in `dashboard/`. The dashboard ingress lives in `internal/dashboard`.
- Hugo pages live in `docs/content/docs/`. Navigation lives in `docs/data/navigation.toml`. Keep diagrams and their editable sources together.

## Validate

Run checks for the changed layer. Use `make docs-build` for docs and `make check` for the full source check. Use the relevant live acceptance procedure for changes to identity, runtime isolation, or storage. The [check matrix](docs/content/docs/development/_index.md#check-your-changes) maps change types to commands.
