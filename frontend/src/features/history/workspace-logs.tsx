import { logsOptions, type Logs } from "./logs-api";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { Modal, ErrorNotice, Observation } from "@/shared/components";
import { label } from "@/shared/format";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import type { Project, Workspace } from "@/shared/types";

function sourceLabel(source: string) {
  return source === "workspace"
    ? "Worker"
    : source === "t3"
      ? "T3"
      : label(source);
}

export function eventText(
  e: Pick<Logs["items"][number], "kind" | "tool" | "status">,
) {
  const text: Record<string, string> = {
    "task.queued": "Task queued",
    "task.started": "Task started",
    "task.succeeded": "Task succeeded",
    "task.failed": "Task failed",
    "task.cancelled": "Task cancelled",
    "agent.started": "Agent started",
    "agent.completed": "Agent completed",
    "agent.failed": "Agent failed",
    "tool.started": "Tool started",
    "tool.completed": "Tool completed",
    "service.started": "Listener started",
    "service.connected": "Listener connected",
    "service.stopped": "Listener stopped",
    "service.failed": "Listener failed",
    "message.received": "Message received",
    "response.started": "Preparing response",
    "response.completed": "Response prepared",
    "response.failed": "Response failed",
    "workspace.state": "Worker state",
    "agent.state": "Agent state",
  };
  return (
    (text[e.kind] || "Activity") +
    (e.tool ? ` · ${e.tool}` : "") +
    (e.status ? ` · ${e.status}` : "")
  );
}
export function WorkspaceLogs({
  project,
  workspace,
  workspaces,
  onClose,
}: {
  project: Project;
  workspace?: Workspace;
  workspaces: Workspace[];
  onClose: () => void;
}) {
  const [selected, setSelected] = useState(workspace?.id || "");
  const [live, setLive] = useState(true);
  const [source, setSource] = useState("");
  const query = useQuery({
    ...logsOptions(project.id, selected),
    refetchInterval: live ? 2000 : false,
    refetchOnWindowFocus: live,
    refetchOnReconnect: live,
  });
  const items = (query.data?.items || []).filter(
    (e) => !source || e.source === source,
  );
  const names = new Map(workspaces.map((w) => [w.id, w.name || w.id]));
  return (
    <Modal title="Worker activity logs" onClose={onClose} wide>
      <div className="flex flex-wrap gap-3 items-center">
        <NativeSelect
          aria-label="Worker log filter"
          wrapperClassName="w-full sm:w-auto sm:flex-1"
          value={selected}
          onChange={(e) => setSelected(e.target.value)}
        >
          <option value="">All workers in this project</option>
          {workspaces.map((w) => (
            <option key={w.id} value={w.id}>
              {w.name || w.id}
            </option>
          ))}
        </NativeSelect>
        <NativeSelect
          aria-label="Log source"
          wrapperClassName="w-full sm:w-auto"
          value={source}
          onChange={(e) => setSource(e.target.value)}
        >
          <option value="">All sources</option>
          {["workspace", "codex", "claude", "hermes", "matrix", "t3"].map(
            (s) => (
              <option key={s} value={s}>
                {sourceLabel(s)}
              </option>
            ),
          )}
        </NativeSelect>
        <Button variant="outline" onClick={() => setLive((v) => !v)}>
          {live ? "Pause updates" : "Resume updates"}
        </Button>
        <Button
          variant="outline"
          disabled={query.isFetching}
          onClick={() => query.refetch()}
        >
          Refresh
        </Button>
      </div>
      <p className="text-sm text-muted-foreground">
        {live ? "Updates every 2 seconds." : "Updates paused."} Shows the latest
        500 recorded events. Older events are retained up to 2,000 per worker.
        Prompts, commands, tool output, credentials and reasoning are excluded.
      </p>
      <Observation
        at={query.dataUpdatedAt}
        fetching={query.isFetching}
        error={query.isError}
      />
      {selected && query.data && !query.isError && (
        <div className="rounded-lg border p-3 text-sm space-y-1">
          <p>
            {label(query.data.state || "unknown")} ·{" "}
            {query.data.active_runs ?? "—"}{" "}
            {query.data.active_runs === 1 ? "run" : "runs"} ·{" "}
            {query.data.background_jobs ?? "—"} background{" "}
            {query.data.background_jobs === 1 ? "process" : "processes"}
          </p>
          {query.data.services?.map((s) => (
            <p key={s.name}>
              {s.name}: <strong>{s.state}</strong>
            </p>
          ))}
        </div>
      )}
      {query.isError && (
        <ErrorNotice
          error={query.error}
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      )}
      <div
        className="max-h-[55vh] overflow-auto rounded-lg border"
        role="log"
        aria-label="Worker activity events"
        aria-live="off"
        tabIndex={0}
      >
        {query.isPending ? (
          <p role="status" className="p-4">
            Loading activity…
          </p>
        ) : items.length === 0 && !query.isError ? (
          <p className="p-4 text-sm text-muted-foreground">
            {source
              ? "No recorded events match this source. Choose All sources to see other activity."
              : "No activity recorded yet. Events appear as agents work; opening this view does not start a worker."}
          </p>
        ) : items.length ? (
          <table className="block w-full text-left text-sm sm:table">
            <thead className="sr-only sm:not-sr-only sm:sticky sm:top-0 bg-background">
              <tr>
                <th className="p-3">Time</th>
                <th className="p-3">Worker / source</th>
                <th className="p-3">Activity</th>
              </tr>
            </thead>
            <tbody className="block sm:table-row-group">
              {items.map((e) => (
                <tr
                  key={e.workspace_id + e.id}
                  className="grid grid-cols-2 border-t sm:table-row"
                >
                  <td
                    className="block p-3 whitespace-nowrap font-mono text-xs sm:table-cell"
                    title={e.at}
                  >
                    {new Date(e.at).toLocaleTimeString()}
                  </td>
                  <td className="block min-w-0 p-3 sm:table-cell">
                    <span>{names.get(e.workspace_id) || e.workspace_id}</span>
                    <span className="block text-xs text-muted-foreground">
                      {sourceLabel(e.source)}
                    </span>
                  </td>
                  <td className="block min-w-0 col-span-2 px-3 pb-3 sm:table-cell sm:p-3">
                    {eventText(e)}
                    {e.task_id && (
                      <span className="block font-mono text-xs text-muted-foreground">
                        {e.task_id}
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : null}
      </div>
    </Modal>
  );
}
