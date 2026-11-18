import { accessBlocked } from "@/shared/query-policy";
import { useSearch } from "@tanstack/react-router";
import {
  BulkWorkerAction,
  WorkerSelection,
  type WorkerBatch,
} from "@/features/platform/worker-actions";
import { lazy, Suspense } from "react";
import { DeleteWorker } from "./delete-worker";
import { StartWorker } from "./start-worker";
import { WorkerRow } from "./worker-row";
import { WorkspaceSettings } from "./settings";
import { CopyForm } from "./file-handoff";
import { PairingPanel } from "./pairing-panel";
import { WorkspaceForm } from "./workspace-form";
import { workspaceOptions } from "./api";
import { WorkspaceHistory } from "@/features/history/workspace-history";
import { WorkspaceLogs } from "@/features/history/workspace-logs";
import { Plus, Search, RefreshCw, MoreHorizontal, Copy } from "lucide-react";
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
import { bytes } from "@/shared/format";
import { Empty, ErrorNotice, Observation } from "@/shared/components";
import type { Pairing, Project, Workspace } from "@/shared/types";

const WorkerTerminal = lazy(() =>
  import("./worker-terminal").then((module) => ({
    default: module.WorkerTerminal,
  })),
);

export function Workspaces({ project }: { project: Project }) {
  const client = useQueryClient();
  const query = useQuery(workspaceOptions(project.id));
  const blocked = accessBlocked(query.error);
  const globalProfiles = useConnectionProfiles(
    !blocked && !!query.data?.length,
  );
  const [bulk, setBulk] = useState(false);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [batch, setBatch] = useState<WorkerBatch>();
  const routeSearch = useSearch({ strict: false });
  const [search, setSearch] = useState(routeSearch.worker || "");
  const [view, setView] = useState<"all" | "running" | "stopped">("all");
  const [create, setCreate] = useState(false);
  const [parent, setParent] = useState<Workspace>();
  const [copy, setCopy] = useState(false);
  const [start, setStart] = useState<Workspace>();
  const [deleting, setDeleting] = useState<Workspace>();
  const [pairing, setPairing] = useState<{
    workspace: Workspace;
    value: Pairing;
  }>();
  const [settings, setSettings] = useState<Workspace>();
  const [terminal, setTerminal] = useState<Workspace>();
  const [logs, setLogs] = useState<Workspace | "all">();
  const [history, setHistory] = useState<Workspace | "all">();
  const [message, setMessage] = useState("");
  const [actionError, setActionError] = useState<unknown>();
  const [busy, setBusy] = useState("");
  const workspaces = blocked ? [] : query.data || [];
  const sharedWorkspace = workspaces.some((workspace) => workspace.shared);
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
    if (action === "delete") {
      setDeleting(workspace);
      return;
    }
    const tab = open ? window.open("about:blank", "_blank") : null;
    if (tab) tab.opener = null;
    setBusy(workspace.id);
    setActionError(undefined);
    setMessage("");
    try {
      if (action === "enable-t3") {
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
            location.origin,
          ),
        });
      } else if (action === "stop") {
        await workspacesApi.stop(workspace.id);
        setMessage(
          (workspace.name || "Worker") +
            " stopped. Files and conversation history are preserved.",
        );
      } else {
        const result = await workspacesApi.start(workspace.id);
        if (open) {
          if (!result.console_ticket)
            throw new Error(
              "The worker started, but its browser connection was not available. Try Open T3 again. If this repeats, ask your administrator to check the T3 gateway.",
            );
          const port = import.meta.env.DEV
            ? import.meta.env.VITE_WORKBENCH_PORT || "8080"
            : location.port;
          const url = consoleURL(
            workspace.id,
            result.console_ticket,
            port,
            location.origin,
          );
          if (tab) tab.location.replace(url);
          else
            setMessage(
              "Worker started. Allow popups, then select Open T3 again.",
            );
        } else setMessage((workspace.name || "Worker") + " started.");
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
          <div className="flex items-center gap-2">
            <h1>Workers</h1>
            {query.data && (
              <Badge variant="secondary" className="rounded-full">
                {workspaces.length}
              </Badge>
            )}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="outline"
                aria-label="Project actions"
                disabled={blocked}
                className="max-sm:size-8 max-sm:p-0"
              >
                <MoreHorizontal />
                <span className="hidden sm:inline">Project actions</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => setLogs("all")}>
                Activity logs
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => setHistory("all")}>
                Message history
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                disabled={workspaces.length < 2}
                onSelect={() => setCopy(true)}
              >
                <Copy />
                Copy a file
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          {!sharedWorkspace && (
            <Button disabled={blocked} onClick={() => setCreate(true)}>
              <Plus className="size-4" />
              Create worker
            </Button>
          )}
        </div>
      </div>
      {sharedWorkspace && (
        <p className="text-sm text-muted-foreground">
          All projects use this shared workspace. Files, history and connection
          settings are shared.
        </p>
      )}
      {query.isError && (
        <ErrorNotice
          error={query.error}
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      )}{" "}
      {actionError != null && <ErrorNotice error={actionError} />}{" "}
      {message && (
        <div className="notice" role="status">
          {message}
        </div>
      )}
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="inline-flex min-h-8 max-w-full flex-wrap items-center border bg-background p-0.5">
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
              className="h-6 gap-2 px-3 text-xs"
              onClick={() => {
                setView(value);
                setChecked(new Set());
              }}
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
        <div className="flex min-w-0 max-w-full flex-wrap items-center gap-2">
          <Button
            variant="outline"
            aria-expanded={bulk}
            onClick={() => {
              setBulk(!bulk);
              setChecked(new Set());
            }}
          >
            {bulk ? "Done" : "Bulk operations"}
          </Button>
          <div className="compact-search">
            <Search size={14} />
            <Input
              aria-label="Search workers"
              placeholder="Find a worker…"
              className="h-8 pl-8 text-xs md:text-xs shadow-none"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setChecked(new Set());
              }}
            />
          </div>
          <Button
            variant="outline"
            size="icon"
            className="size-8"
            aria-label="Refresh workers"
            disabled={query.isFetching}
            onClick={() => refresh()}
          >
            <RefreshCw
              className={cn("size-4", query.isFetching && "loading-refresh")}
            />
          </Button>
        </div>
      </div>
      {bulk && (
        <WorkerSelection
          visible={visible}
          checked={checked}
          onChange={setChecked}
          onAction={setBatch}
        />
      )}
      {batch && (
        <BulkWorkerAction
          batch={batch}
          projects={[project]}
          onClose={() => setBatch(undefined)}
          onSucceeded={(id) =>
            setChecked((current) => {
              const next = new Set(current);
              next.delete(id);
              return next;
            })
          }
        />
      )}
      {blocked ? null : query.isPending ? (
        <div className="loading-placeholder space-y-3 rounded-md border bg-card p-4">
          {[1, 2, 3, 4].map((i) => (
            <Skeleton key={i} className="h-16 w-full" />
          ))}
        </div>
      ) : !query.data ? null : workspaces.length === 0 ? (
        <Empty
          title="Your project's first worker"
          action={
            <Button onClick={() => setCreate(true)}>
              <Plus />
              Create worker
            </Button>
          }
        >
          Choose an agent and a connection profile. Files and history stay with
          this project when the worker stops.
        </Empty>
      ) : (
        <div className="workspace-inventory overflow-hidden border bg-background">
          <Table
            aria-label="Workers"
            className="w-full md:min-w-[960px] md:table-fixed"
          >
            <caption className="sr-only">
              Worker state, resources, network access and controls
            </caption>
            <TableHeader className="max-md:hidden">
              <TableRow className="bg-muted/15 hover:bg-muted/15">
                {bulk && (
                  <TableHead className="w-12">
                    <span className="sr-only">Select worker</span>
                  </TableHead>
                )}
                <TableHead className="w-[28%] pl-5">Worker</TableHead>
                <TableHead className="w-[14%]">Status</TableHead>
                <TableHead className="w-[18%]">Allocation</TableHead>
                <TableHead className="w-[20%]">Connections</TableHead>
                <TableHead className="w-[20%] pr-5 text-right">
                  Actions
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody className="max-md:block">
              {visible.map((workspace) => (
                <WorkerRow
                  key={workspace.id}
                  workspace={workspace}
                  selection={
                    bulk
                      ? {
                          checked: checked.has(workspace.id),
                          toggle: () =>
                            setChecked((current) => {
                              const next = new Set(current);
                              if (next.has(workspace.id))
                                next.delete(workspace.id);
                              else next.add(workspace.id);
                              return next;
                            }),
                        }
                      : undefined
                  }
                  profiles={globalProfiles.data || []}
                  busy={busy}
                  operate={operate}
                  setStart={setStart}
                  setLogs={setLogs}
                  setHistory={setHistory}
                  setSettings={setSettings}
                  setTerminal={setTerminal}
                  setParent={setParent}
                  setMessage={setMessage}
                  setActionError={setActionError}
                />
              ))}
              {!visible.length && (
                <TableRow>
                  <TableCell
                    colSpan={bulk ? 6 : 5}
                    className="h-32 text-center text-muted-foreground"
                  >
                    No workers match this filter.{" "}
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
          <div className="flex flex-wrap items-center justify-between gap-2 border-t bg-muted/15 px-5 py-2 text-[11px] text-muted-foreground">
            <span>
              {visible.length} of {workspaces.length}{" "}
              {workspaces.length === 1 ? "worker" : "workers"}
            </span>
            <Observation
              at={query.dataUpdatedAt}
              fetching={query.isFetching}
              error={query.isError}
            />
          </div>
        </div>
      )}
      {deleting && (
        <DeleteWorker
          workspace={workspaces.find((w) => w.id === deleting.id) || deleting}
          onClose={() => setDeleting(undefined)}
          onDeleted={() => {
            setDeleting(undefined);
            setMessage(
              "Worker deleted from Workbench. Private disks retained.",
            );
            void refresh();
          }}
        />
      )}
      {start && (
        <StartWorker
          workspace={start}
          onClose={() => setStart(undefined)}
          onStarted={() => {
            setMessage(`${start.name || "Worker"} started.`);
            setStart(undefined);
            refresh();
          }}
          onSettings={() => {
            setSettings(start);
            setStart(undefined);
          }}
        />
      )}
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
          onSaved={() => {
            setSettings(undefined);
            refresh();
            setMessage("Worker settings saved.");
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
      {terminal && (
        <Suspense fallback={<span role="status">Loading terminal…</span>}>
          <WorkerTerminal
            workspace={terminal}
            onClose={() => setTerminal(undefined)}
          />
        </Suspense>
      )}
      {logs && (
        <WorkspaceLogs
          project={project}
          workspaces={workspaces}
          workspace={logs === "all" ? undefined : logs}
          onClose={() => setLogs(undefined)}
        />
      )}
    </>
  );
}
