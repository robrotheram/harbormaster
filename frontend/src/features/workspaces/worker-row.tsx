import {
  GitBranch,
  Globe,
  Network,
  Play,
  ExternalLink,
  Loader2,
  Monitor,
  MoreHorizontal,
  Settings2,
  Copy,
  Square,
  TerminalSquare,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { TableCell, TableRow } from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { WorkspaceActivity } from "@/features/history/workspace-activity";
import { bytes, date } from "@/shared/format";
import type { Workspace, GlobalConnectionProfile } from "@/shared/types";

export type WorkerOperation =
  "start" | "stop" | "pair" | "enable-t3" | "delete";
export function WorkerRow({
  workspace,
  profiles,
  busy,
  operate,
  setStart,
  setLogs,
  setHistory,
  setSettings,
  setTerminal,
  setParent,
  setMessage,
  setActionError,
}: {
  workspace: Workspace;
  profiles: GlobalConnectionProfile[];
  busy: string;
  operate: (
    workspace: Workspace,
    action: WorkerOperation,
    open?: boolean,
  ) => void;
  setStart: (workspace: Workspace) => void;
  setLogs: (workspace: Workspace) => void;
  setHistory: (workspace: Workspace) => void;
  setSettings: (workspace: Workspace) => void;
  setTerminal: (workspace: Workspace) => void;
  setParent: (workspace: Workspace) => void;
  setMessage: (message: string) => void;
  setActionError: (error: unknown) => void;
}) {
  const console = workspace.console_enabled || workspace.agent_kind === "t3";
  const kind = workspace.agent_kind || "codex";
  const stopped = ["STOPPED", "ERROR", "UNKNOWN"].includes(workspace.state);
  const transitioning = ["STARTING", "STOPPING"].includes(workspace.state);
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
          onOpenActivity={() => setLogs(workspace)}
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
        {workspace.github_access &&
          !workspace.git_proxy_ready &&
          workspace.state !== "STOPPED" && (
            <div className="mt-1.5 text-[11px] text-amber-700 dark:text-amber-400">
              Proxy update: stop/start required
            </div>
          )}
      </TableCell>
      <TableCell className="pr-5 max-md:col-span-2 max-md:px-0 max-md:pt-3 max-md:pb-0">
        <div className="flex items-center justify-end gap-1.5 max-md:justify-start">
          {(stopped || transitioning || console) && (
            <Button
              size="sm"
              variant="outline"
              disabled={
                !!busy || transitioning || (!stopped && !workspace.ready)
              }
              onClick={() =>
                stopped
                  ? setStart(workspace)
                  : operate(workspace, "start", true)
              }
            >
              {busy === workspace.id ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : stopped ? (
                <Play className="size-3.5" />
              ) : (
                <ExternalLink className="size-3.5" />
              )}
              {busy === workspace.id
                ? "Working…"
                : transitioning
                  ? workspace.state === "STARTING"
                    ? "Starting…"
                    : "Stopping…"
                  : stopped
                    ? "Check & start"
                    : "Open T3"}
            </Button>
          )}
          {workspace.ready && (
            <Button
              size="sm"
              variant="outline"
              disabled={!!busy || transitioning}
              onClick={() => setTerminal(workspace)}
            >
              <TerminalSquare className="size-3.5" /> Open terminal
            </Button>
          )}
          {console && workspace.ready ? (
            <Button
              variant="outline"
              size="icon-sm"
              title="Connect T3 desktop"
              aria-label="Connect T3 desktop"
              disabled={!!busy}
              onClick={() => operate(workspace, "pair")}
            >
              <Monitor className="size-3.5" />
            </Button>
          ) : null}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={"Actions for " + (workspace.name || workspace.id)}
                disabled={!!busy}
              >
                <MoreHorizontal />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-52">
              <DropdownMenuLabel>Worker actions</DropdownMenuLabel>
              <DropdownMenuItem onSelect={() => setLogs(workspace)}>
                <TerminalSquare />
                Activity logs
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => setHistory(workspace)}>
                <TerminalSquare />
                Message history
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => setSettings(workspace)}>
                <Settings2 />
                Settings
              </DropdownMenuItem>
              {!console && kind === "codex" && workspace.ready && (
                <DropdownMenuItem
                  onSelect={() => operate(workspace, "enable-t3")}
                >
                  <Monitor />
                  Enable T3
                </DropdownMenuItem>
              )}
              <DropdownMenuItem
                disabled={workspace.shared}
                onSelect={() => setParent(workspace)}
              >
                <GitBranch />
                Branch worker
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={() =>
                  navigator.clipboard
                    .writeText(workspace.id)
                    .then(() => setMessage("Worker ID copied."))
                    .catch(setActionError)
                }
              >
                <Copy />
                Copy worker ID
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              {workspace.state !== "STOPPED" ? (
                <DropdownMenuItem
                  disabled={workspace.state !== "READY"}
                  onSelect={() => operate(workspace, "stop")}
                  className="text-destructive focus:text-destructive"
                >
                  <Square />
                  Stop worker
                </DropdownMenuItem>
              ) : (
                <DropdownMenuItem onSelect={() => setStart(workspace)}>
                  <Play />
                  Start worker
                </DropdownMenuItem>
              )}
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-destructive focus:text-destructive"
                disabled={workspace.shared}
                onSelect={() => operate(workspace, "delete")}
              >
                Delete worker
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
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
  if (p?.mode === "allowlist")
    return "Allowlist · " + p.destinations.length + " destinations";
  return workspace.github_access
    ? workspace.git_proxy_ready
      ? "GitHub allowed"
      : "GitHub enabled — runtime update pending"
    : "External connections blocked";
}
