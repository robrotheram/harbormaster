# Global tool profiles

Open **Platform → Tool profiles** to create or edit a reusable profile. Set its
name, description, and the bundled MCP servers to expose:

- **Workspace files**: read, write and list workspace files.
- **Project Git**: read Git status, diffs and recent commits.

An empty selection is supported. Choose the profile when creating a workspace;
the existing Development, Vulnerability research, Banking project and Photo
gallery presets remain editable with their original IDs. Profile changes apply
on the next stop/start, preserving running work. Concurrent edits are protected
by revision checks. Only the operator can edit global profiles.

Profiles configure the bundled MCP servers in T3 and native Codex/Claude task
workers. They do not disable built-in agent capabilities such as terminal tools,
change network permissions, or install arbitrary commands/remote MCP servers.
Hermes uses its own native toolsets. Sandbox isolation and connection profiles
continue to control access independently.

The control plane stores these profiles as global metadata, seeds defaults only
when missing, and resolves their latest tools at workspace start. The runtime
accepts only the two bundled server IDs. Explicit empty lists differ from a
missing legacy selection, which retains both default MCP servers. Applied
selections are passed to the guest at boot; managed T3 MCP sections are updated
without overwriting provider configuration. Docker recreates a stopped container
when its selection changes, retaining its owned data volume.

`GET /v1/tool-profiles` lists profiles; `POST /v1/tool-profiles` creates one using
`title`, `description`, `mcps` and `revision: 0`. Update with
`POST /v1/tool-profiles/{id}` and its current revision. `GET /v1/profiles` remains
the compatible workspace selector endpoint backed by the same durable data.
