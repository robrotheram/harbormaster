import { WorkerCheckbox } from "@/features/platform/worker-actions";
import { GitBranch, Globe, Network } from "lucide-react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { TableCell, TableRow } from "@/components/ui/table";
import { isGitHubOnly } from "@/features/connection-profiles/policy";
import { WorkspaceActivity } from "@/features/history/workspace-activity";
import { bytes, date } from "@/shared/format";
import type { Workspace, GlobalConnectionProfile } from "@/shared/types";

export function WorkerRow({
  workspace,
  projectID,
  selection,
  profiles,
}: {
  workspace: Workspace;
  projectID: string;
  selection?: { checked: boolean; toggle: () => void };
  profiles: GlobalConnectionProfile[];
}) {
  const navigate = useNavigate();
  const console = workspace.console_enabled || workspace.agent_kind === "t3";
  const kind = workspace.agent_kind || "codex";
  const linked = profiles.find((p) => p.id === workspace.connection_profile_id);
  const pending =
    linked && linked.revision > (workspace.connection_profile_revision ?? 0);
  const open = workspace.connections?.mode === "open";
  return (
    <TableRow
      key={workspace.id}
      data-workspace={workspace.id}
      className="max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto] max-md:gap-x-2 max-md:p-4"
    >
      {selection && (
        <TableCell>
          <WorkerCheckbox
            label={"Select worker " + (workspace.name || workspace.id)}
            checked={selection.checked}
            onChange={selection.toggle}
          />
        </TableCell>
      )}
      <TableCell className="py-3 pl-5 max-md:min-w-0 max-md:p-0">
        <div className="min-w-0">
          <div
            className="truncate text-xs font-medium"
            title={`${workspace.name || "Untitled worker"} (${workspace.id})`}
          >
            {workspace.name || "Untitled worker"}
          </div>
          <div className="mt-1.5 flex min-w-0 items-center gap-2 text-[11px] text-muted-foreground">
            <span className="shrink-0">
              {kind === "hermes" ? "Hermes" : console ? "T3 / Codex" : "Codex"}
            </span>
            {!workspace.name && (
              <span className="truncate" title={workspace.id}>
                {workspace.id}
              </span>
            )}
            {workspace.branch && (
              <span
                className="flex min-w-0 items-center gap-1"
                title={workspace.branch}
              >
                <GitBranch className="size-3 shrink-0" />
                <span className="truncate">{workspace.branch}</span>
              </span>
            )}
          </div>
        </div>
      </TableCell>
      <TableCell className="max-md:p-0 max-md:text-right">
        <WorkspaceActivity
          workspace={workspace}
          onOpenActivity={() =>
            void navigate({
              to: "/projects/$projectId/workspaces/$workerId",
              params: { projectId: projectID, workerId: workspace.id },
              search: { panel: "activity" },
            })
          }
        />
      </TableCell>
      <TableCell className="max-md:col-span-2 max-md:flex max-md:flex-wrap max-md:items-baseline max-md:gap-x-4 max-md:px-0 max-md:pt-3 max-md:pb-2">
        <div className="text-xs">
          {workspace.cpu_limit} CPU{" "}
          <span className="mx-1 text-muted-foreground">/</span>{" "}
          {bytes(workspace.memory_limit_bytes)}
        </div>
        <div className="mt-1.5 text-[11px] text-muted-foreground">
          {workspace.runtime_type === "firecracker"
            ? "Firecracker"
            : workspace.runtime_type || "Runtime unreported"}
          {workspace.ready && workspace.memory_bytes != null && (
            <span> · {bytes(workspace.memory_bytes)} used</span>
          )}
        </div>
      </TableCell>
      <TableCell className="max-md:col-span-2 max-md:px-0 max-md:py-2">
        <div className="flex items-center gap-1.5 text-xs font-medium">
          {open ? (
            <Globe className="size-3.5 text-amber-600" />
          ) : (
            <Network className="size-3.5 text-muted-foreground" />
          )}
          <span
            className="max-w-44 truncate"
            title={connectionLabel(workspace)}
          >
            {linked?.name || connectionLabel(workspace)}
          </span>
        </div>
        <div className="mt-1.5 text-[11px] text-muted-foreground">
          {workspace.connection_profile_id
            ? `Global profile · v${workspace.connection_profile_revision}`
            : "Worker-specific"}
        </div>
        {pending && (
          <div className="mt-1.5 text-[11px] text-amber-700 dark:text-amber-400">
            Profile update pending
          </div>
        )}
      </TableCell>
      <TableCell className="pr-5 max-md:col-span-2 max-md:px-0 max-md:pt-3 max-md:pb-0">
        <div className="flex flex-wrap items-center justify-end gap-1.5 max-md:justify-start">
          <Button asChild variant="outline" size="sm">
            <Link
              to="/projects/$projectId/workspaces/$workerId"
              params={{ projectId: projectID, workerId: workspace.id }}
              aria-label={`Open worker ${workspace.name || workspace.id}`}
            >
              Open worker
            </Link>
          </Button>
        </div>
        {workspace.ready && (
          <div className="mt-1.5 truncate text-right text-[10px] text-muted-foreground max-md:text-left">
            {workspace.idle_timeout_minutes === 0
              ? "Auto-stop off"
              : workspace.idle_stop_at
                ? "Idle stop " + date(workspace.idle_stop_at)
                : "Idle timeout " +
                  (workspace.idle_timeout_minutes ?? 60) +
                  " min"}
          </div>
        )}
      </TableCell>
    </TableRow>
  );
}

function connectionLabel(workspace: Workspace) {
  const p = workspace.connections;
  if (p?.mode === "open") return "Fully open · unrestricted network";
  if (isGitHubOnly(p)) return "GitHub allowed";
  if (p?.mode === "allowlist")
    return "Allowlist · " + p.destinations.length + " destinations";
  return "External connections blocked";
}
