import { WorkspaceSettings } from "./settings";
import { CopyForm } from "./file-handoff";
import { PairingPanel } from "./pairing-panel";
import { WorkspaceForm } from "./workspace-form";
import { workspaceOptions } from "./api";
import { TaskPanel } from "@/features/tasks/task-panel";
import { WorkspaceActivity } from "@/features/history/workspace-activity";
import { WorkspaceHistory } from "@/features/history/workspace-history";
import { WorkspaceLogs } from "@/features/history/workspace-logs";
import {
  Plus,
  Search,
  RefreshCw,
  TerminalSquare,
  GitBranch,
  Cpu,
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
  Files,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useState } from "react";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { consoleURL, safePairing } from "@/features/workspaces/connections";
import { keys } from "@/shared/query-keys";

import { workspacesApi } from "@/features/workspaces/api";
import { useConnectionProfiles } from "@/features/connection-profiles/api";
import { bytes, date } from "@/shared/format";
import {
  Empty,
  ErrorNotice,
  Observation,
  WorkspaceName,
} from "@/shared/components";
import type { Pairing, Project, Workspace } from "@/shared/types";

export function Workspaces({ project }: { project: Project }) {
  const client = useQueryClient();
  const query = useQuery(workspaceOptions(project.id));
  const globalProfiles = useConnectionProfiles();
  const [search, setSearch] = useState("");
  const [view, setView] = useState<"all" | "running" | "stopped">("all");
  const [create, setCreate] = useState(false);
  const [parent, setParent] = useState<Workspace>();
  const [copy, setCopy] = useState(false);
  const [pairing, setPairing] = useState<{
    workspace: Workspace;
    value: Pairing;
  }>();
  const [settings, setSettings] = useState<Workspace>();
  const [task, setTask] = useState<Workspace>();
  const [logs, setLogs] = useState<Workspace | "all">();
  const [history, setHistory] = useState<Workspace | "all">();
  const [message, setMessage] = useState("");
  const [actionError, setActionError] = useState<unknown>();
  const [busy, setBusy] = useState("");
  const workspaces = query.data || [];
  const visible = workspaces.filter(
    (w) =>
      (view === "all" ||
        (view === "stopped"
          ? w.state === "STOPPED"
          : ["READY", "BUSY", "STARTING", "STOPPING"].includes(w.state))) &&
      [w.name, w.id, w.branch].some((v) =>
        v?.toLowerCase().includes(search.toLowerCase()),
      ),
  );
  const refresh = () =>
    client.invalidateQueries({ queryKey: keys.project(project.id) });
  const changed = () => {
    setCreate(false);
    setParent(undefined);
    refresh();
  };

  async function operate(
    workspace: Workspace,
    action: "start" | "stop" | "pair" | "enable-t3" | "delete",
    open = false,
  ) {
    if (busy) return;
    const tab = open ? window.open("about:blank", "_blank") : null;
    if (tab) tab.opener = null;
    setBusy(workspace.id);
    setActionError(undefined);
    setMessage("");
    try {
      if (action === "delete") {
        if (
          !window.confirm(
            `Delete ${workspace.name || workspace.id} from the workbench? Compute must stop first. Private disks remain on the host.`,
          )
        )
          return;
        await workspacesApi.delete(workspace.id);
        setMessage(
          "Workspace deleted from the workbench. Private disks retained.",
        );
      } else if (action === "enable-t3") {
        await workspacesApi.enableConsole(workspace.id);
        setMessage(
          "T3 enabled. You can now open the browser or connect T3 desktop.",
        );
      } else if (action === "pair") {
        setPairing({
          workspace,
          value: safePairing(
            await workspacesApi.pair(workspace.id),
            workspace.id,
          ),
        });
      } else if (action === "stop") {
        await workspacesApi.stop(workspace.id);
        setMessage(
          (workspace.name || "Workspace") +
            " stopped. Files and conversation history are preserved.",
        );
      } else {
        const result = await workspacesApi.start(workspace.id);
        if (open) {
          if (!result.console_ticket)
            throw new Error(
              "The workspace started but no browser connection ticket was returned.",
            );
          const port = import.meta.env.DEV
            ? import.meta.env.VITE_WORKBENCH_PORT || "8080"
            : location.port;
          const url = consoleURL(workspace.id, result.console_ticket, port);
          if (tab) tab.location.replace(url);
          else
            setMessage(
              "Workspace started. Allow popups, then select Open T3 again.",
            );
        } else setMessage((workspace.name || "Workspace") + " started.");
      }
    } catch (error) {
      tab?.close();
      setActionError(error);
    } finally {
      setBusy("");
      refresh();
    }
  }

  return (
    <>
      <div className="page-title">
        <div>
          <div className="mb-2 flex items-center gap-2">
            <h1>Workspaces</h1>
            {query.data && (
              <Badge variant="secondary" className="rounded-full">
                {workspaces.length}
              </Badge>
            )}
          </div>
          <p>Run your agents in dedicated environments for {project.name}.</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => setLogs("all")}>
            Activity logs
          </Button>
          <Button variant="outline" onClick={() => setHistory("all")}>
            Message history
          </Button>
          <Button onClick={() => setCreate(true)}>
            <Plus className="size-4" />
            Create workspace
          </Button>
        </div>
      </div>
      {query.isError && <ErrorNotice error={query.error} />}{" "}
      {actionError != null && <ErrorNotice error={actionError} />}{" "}
      {message && (
        <div className="notice" role="status">
          {message}
        </div>
      )}
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="inline-flex rounded-lg border bg-background p-1">
          {(
            [
              ["all", "All"],
              ["running", "Running"],
              ["stopped", "Stopped"],
            ] as const
          ).map(([value, text]) => (
            <Button
              key={value}
              variant={view === value ? "secondary" : "ghost"}
              size="sm"
              className="h-7 gap-2 px-3 text-xs"
              onClick={() => setView(value)}
              aria-pressed={view === value}
            >
              {text}
              <span className="text-muted-foreground">
                {query.data
                  ? workspaces.filter(
                      (w) =>
                        value === "all" ||
                        (value === "stopped"
                          ? w.state === "STOPPED"
                          : ["READY", "BUSY", "STARTING", "STOPPING"].includes(
                              w.state,
                            )),
                    ).length
                  : "—"}
              </span>
            </Button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-2.5 size-3.5 text-muted-foreground" />
            <Input
              aria-label="Search workspaces"
              placeholder="Find a workspace…"
              className="h-9 w-52 bg-background pl-9 sm:w-64"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <Button
            variant="outline"
            size="icon"
            aria-label="Refresh workspaces"
            disabled={query.isFetching}
            onClick={() => refresh()}
          >
            <RefreshCw
              className={cn(
                "size-4",
                query.isFetching && "motion-safe:animate-spin",
              )}
            />
          </Button>
        </div>
      </div>
      {query.isPending ? (
        <div className="space-y-3 rounded-xl border bg-background p-5">
          {[1, 2, 3, 4].map((i) => (
            <Skeleton key={i} className="h-16 w-full" />
          ))}
        </div>
      ) : !query.data ? null : workspaces.length === 0 ? (
        <Empty
          title="Your project's first workspace"
          action={
            <Button onClick={() => setCreate(true)}>
              <Plus />
              Create workspace
            </Button>
          }
        >
          Choose an agent and a connection profile. Files and history stay with
          this project when the workspace stops.
        </Empty>
      ) : (
        <div className="overflow-hidden rounded-xl border bg-background">
          <Table aria-label="Workspaces" className="w-full md:min-w-[1000px]">
            <caption className="sr-only">
              Workspace state, resources, network access and controls
            </caption>
            <TableHeader className="max-md:hidden">
              <TableRow className="bg-muted/40 hover:bg-muted/40">
                <TableHead className="w-[29%] pl-5">Workspace</TableHead>
                <TableHead className="w-[17%]">Status</TableHead>
                <TableHead>Resources</TableHead>
                <TableHead className="w-[22%]">Connections</TableHead>
                <TableHead className="pr-5 text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody className="max-md:block">
              {visible.map((workspace) => {
                const console =
                  workspace.console_enabled || workspace.agent_kind === "t3";
                const kind = workspace.agent_kind || "codex";
                const stopped =
                  workspace.state === "STOPPED" || workspace.state === "ERROR";
                const linked = globalProfiles.data?.find(
                  (p) => p.id === workspace.connection_profile_id,
                );
                const pending =
                  linked &&
                  linked.revision >
                    (workspace.connection_profile_revision ?? 0);
                const open = workspace.connections?.mode === "open";
                return (
                  <TableRow
                    key={workspace.id}
                    data-workspace={workspace.id}
                    className="max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto] max-md:gap-x-2 max-md:p-4"
                  >
                    <TableCell className="py-5 pl-5 max-md:min-w-0 max-md:p-0">
                      <div className="flex items-start gap-3">
                        <div
                          className={cn(
                            "mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg border",
                            console
                              ? "border-indigo-200/70 bg-indigo-50 text-indigo-600 dark:border-indigo-900 dark:bg-indigo-950 dark:text-indigo-300"
                              : "bg-muted text-muted-foreground",
                          )}
                        >
                          <TerminalSquare className="size-4" />
                        </div>
                        <div className="min-w-0">
                          <WorkspaceName
                            name={workspace.name}
                            id={workspace.id}
                          />
                          <div className="mt-1.5 flex items-center gap-2 text-xs text-muted-foreground">
                            <span>
                              {console
                                ? "T3 / Codex"
                                : kind === "hermes"
                                  ? "Hermes"
                                  : "Codex worker"}
                            </span>
                            {workspace.branch && (
                              <Badge
                                variant="outline"
                                className="max-w-32 truncate text-[10px]"
                              >
                                <GitBranch className="mr-1 size-2.5" />
                                {workspace.branch}
                              </Badge>
                            )}
                          </div>
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
                      <div className="text-xs font-medium">
                        {bytes(workspace.memory_bytes)}{" "}
                        <span className="font-normal text-muted-foreground">
                          / {bytes(workspace.memory_limit_bytes)}
                        </span>
                      </div>
                      <div className="mt-1.5 flex items-center gap-1 text-[11px] text-muted-foreground">
                        <Cpu className="size-3" />
                        {workspace.cpu_cores == null
                          ? "Unknown"
                          : workspace.cpu_cores.toFixed(2)}{" "}
                        / {workspace.cpu_limit} CPU
                      </div>
                      <div className="mt-1.5 text-[10px] text-muted-foreground">
                        {workspace.runtime_type || "Unknown runtime"} · VM{" "}
                        {workspace.isolation_status || "unverified"}
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
                          : "Workspace-specific"}
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
                      <div className="flex items-center justify-end gap-2 max-md:justify-start">
                        <Button
                          size="sm"
                          disabled={!!busy}
                          onClick={() =>
                            stopped
                              ? operate(workspace, "start")
                              : console
                                ? operate(workspace, "start", true)
                                : setTask(workspace)
                          }
                        >
                          {busy === workspace.id ? (
                            <Loader2 className="size-3.5 animate-spin" />
                          ) : stopped ? (
                            <Play className="size-3.5" />
                          ) : console ? (
                            <ExternalLink className="size-3.5" />
                          ) : (
                            <TerminalSquare className="size-3.5" />
                          )}
                          {busy === workspace.id
                            ? "Working…"
                            : stopped
                              ? "Start workspace"
                              : console
                                ? "Open T3"
                                : "Run task"}
                        </Button>
                        {console && (
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
                        )}
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              aria-label={
                                "Actions for " +
                                (workspace.name || workspace.id)
                              }
                              disabled={!!busy}
                            >
                              <MoreHorizontal />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-52">
                            <DropdownMenuLabel>
                              Workspace actions
                            </DropdownMenuLabel>
                            <DropdownMenuItem
                              onSelect={() => setLogs(workspace)}
                            >
                              <TerminalSquare />
                              Activity logs
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              onSelect={() => setHistory(workspace)}
                            >
                              <TerminalSquare />
                              Message history
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              onSelect={() => setSettings(workspace)}
                            >
                              <Settings2 />
                              Settings
                            </DropdownMenuItem>
                            {console && (
                              <DropdownMenuItem
                                onSelect={() => setTask(workspace)}
                              >
                                <TerminalSquare />
                                Run task
                              </DropdownMenuItem>
                            )}
                            {!console && kind === "codex" && (
                              <DropdownMenuItem
                                onSelect={() => operate(workspace, "enable-t3")}
                              >
                                <Monitor />
                                Enable T3
                              </DropdownMenuItem>
                            )}
                            <DropdownMenuItem
                              onSelect={() => setParent(workspace)}
                            >
                              <GitBranch />
                              Branch workspace
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              onSelect={() =>
                                navigator.clipboard
                                  .writeText(workspace.id)
                                  .then(() =>
                                    setMessage("Workspace ID copied."),
                                  )
                                  .catch(setActionError)
                              }
                            >
                              <Copy />
                              Copy workspace ID
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            {workspace.state !== "STOPPED" ? (
                              <DropdownMenuItem
                                disabled={workspace.state !== "READY"}
                                onSelect={() => operate(workspace, "stop")}
                                className="text-destructive focus:text-destructive"
                              >
                                <Square />
                                Stop workspace
                              </DropdownMenuItem>
                            ) : (
                              <DropdownMenuItem
                                onSelect={() => operate(workspace, "start")}
                              >
                                <Play />
                                Start workspace
                              </DropdownMenuItem>
                            )}
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              className="text-destructive focus:text-destructive"
                              onSelect={() => operate(workspace, "delete")}
                            >
                              Delete workspace
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                      <div className="mt-2 text-right text-[10px] text-muted-foreground max-md:text-left">
                        {workspace.idle_timeout_minutes === 0
                          ? "Auto-stop off"
                          : workspace.idle_stop_at
                            ? "Idle stop " + date(workspace.idle_stop_at)
                            : "Idle timeout " +
                              (workspace.idle_timeout_minutes ?? 60) +
                              " min"}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
              {!visible.length && (
                <TableRow>
                  <TableCell
                    colSpan={5}
                    className="h-32 text-center text-muted-foreground"
                  >
                    No workspaces match this filter.{" "}
                    <Button
                      variant="link"
                      size="sm"
                      onClick={() => {
                        setSearch("");
                        setView("all");
                      }}
                    >
                      Clear filters
                    </Button>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          <div className="flex flex-wrap items-center justify-between gap-2 border-t bg-muted/15 px-5 py-3 text-xs text-muted-foreground">
            <span>
              {visible.length} of {workspaces.length} workspaces
            </span>
            <Observation
              at={query.dataUpdatedAt}
              fetching={query.isFetching}
              error={query.isError}
            />
          </div>
        </div>
      )}
      <section className="mt-6 flex flex-wrap items-center justify-between gap-4 rounded-xl border bg-background p-5">
        <div className="flex items-center gap-3">
          <div className="flex size-9 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <Files className="size-4" />
          </div>
          <div>
            <h2 className="text-sm font-semibold">Share a project file</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Copy selected files between workspaces in this project.
            </p>
          </div>
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={workspaces.length < 2}
          onClick={() => setCopy(true)}
        >
          <Copy className="size-3.5" />
          Copy a file
        </Button>
      </section>
      <details className="security-details">
        <summary>Runtime and provider boundaries</summary>
        <p>
          Ordinary Docker mode does not establish verified VM isolation or
          complete network containment. Codex uses the configured subscription
          path without an automatic API-billing fallback. Project files, private
          agent homes and history remain separate.
        </p>
      </details>
      {(create || parent) && (
        <WorkspaceForm
          project={project}
          parent={parent}
          onClose={() => {
            setCreate(false);
            setParent(undefined);
          }}
          onCreated={changed}
        />
      )}
      {copy && (
        <CopyForm
          project={project}
          workspaces={workspaces}
          onClose={() => setCopy(false)}
          onCopied={(size) => {
            setCopy(false);
            setMessage(
              "File copied (" + bytes(size) + "). The source is unchanged.",
            );
            refresh();
          }}
        />
      )}
      {pairing && (
        <PairingPanel
          workspace={pairing.workspace}
          value={pairing.value}
          onClose={() => setPairing(undefined)}
        />
      )}
      {settings && (
        <WorkspaceSettings
          workspace={settings}
          onClose={() => setSettings(undefined)}
          onSaved={(updated) => {
            setSettings(undefined);
            refresh();
            setMessage(
              updated.github_access &&
                !updated.git_proxy_ready &&
                updated.state !== "STOPPED"
                ? "Settings saved. Finish work, close terminals, then stop/start to apply the proxy update."
                : "Workspace settings saved.",
            );
          }}
        />
      )}
      {history && (
        <WorkspaceHistory
          project={project}
          workspaces={workspaces}
          workspace={history === "all" ? undefined : history}
          onClose={() => setHistory(undefined)}
        />
      )}
      {logs && (
        <WorkspaceLogs
          project={project}
          workspaces={workspaces}
          workspace={logs === "all" ? undefined : logs}
          onClose={() => setLogs(undefined)}
        />
      )}
      {task && (
        <TaskPanel
          project={project}
          workspace={task}
          onClose={() => {
            setTask(undefined);
            refresh();
          }}
        />
      )}
    </>
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
