import type { ConnectionPolicy, Workspace } from "@/shared/types";
export function parseDestinations(value: string): string[] {
  return [
    ...new Set(
      value
        .split(/\r?\n/)
        .map((v) => v.trim().toLowerCase())
        .filter(Boolean),
    ),
  ];
}
export function isGitHubOnly(policy: ConnectionPolicy | undefined) {
  return (
    policy?.mode === "allowlist" &&
    policy.destinations.length === 1 &&
    policy.destinations[0] === "github.com:443"
  );
}
// Presentation only. The control plane remains the authority on allowed changes.
export function connectionChangeNeedsStop(
  workspace: Pick<Workspace, "state" | "connection_mode">,
  policy: ConnectionPolicy,
) {
  return (
    workspace.state !== "STOPPED" &&
    (policy.mode === "open") !== (workspace.connection_mode === "open")
  );
}
