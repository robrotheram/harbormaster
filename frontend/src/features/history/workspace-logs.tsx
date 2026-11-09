import { logsOptions, type Logs } from "./logs-api";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { Modal, ErrorNotice } from "@/shared/components";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import type { Project, Workspace } from "@/shared/types";

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
    "workspace.state": "Workspace state",
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
  });
  const items = (query.data?.items || []).filter(
    (e) => !source || e.source === source,
  );
  const names = new Map(workspaces.map((w) => [w.id, w.name || w.id]));
  return (
    <Modal title="Workspace activity logs" onClose={onClose} wide>
      <div className="flex flex-wrap gap-3 items-center">
        <NativeSelect
          aria-label="Workspace log filter"
          value={selected}
          onChange={(e) => setSelected(e.target.value)}
        >
          <option value="">All workspaces in this project</option>
          {workspaces.map((w) => (
            <option key={w.id} value={w.id}>
              {w.name || w.id}
            </option>
          ))}
        </NativeSelect>
        <NativeSelect
          aria-label="Log source"
          value={source}
          onChange={(e) => setSource(e.target.value)}
        >
          <option value="">All sources</option>
          {["workspace", "codex", "claude", "hermes", "matrix", "t3"].map(
            (s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ),
          )}
        </NativeSelect>
        <Button variant="outline" onClick={() => setLive((v) => !v)}>
          {live ? "Pause updates" : "Resume updates"}
        </Button>
        <Button variant="outline" onClick={() => query.refetch()}>
          Refresh
        </Button>
      </div>
      <p className="text-sm text-muted-foreground">
        {live ? "Updates every 2 seconds." : "Updates paused."} Shows the latest
        500 recorded events. Older events are retained up to 2,000 per
        workspace. Prompts, commands, tool output, credentials and reasoning are
        excluded.
      </p>
      {selected && query.data && (
        <div className="rounded-lg border p-3 text-sm space-y-1">
          <p>
            {query.data.state} · {query.data.active_runs} runs ·{" "}
            {query.data.background_jobs} background processes
          </p>
          {query.data.services?.map((s) => (
            <p key={s.name}>
              {s.name}: <strong>{s.state}</strong>
            </p>
          ))}
        </div>
      )}
      {query.isError && <ErrorNotice error={query.error} />}
      <div
        className="max-h-[55vh] overflow-auto rounded-lg border"
        role="log"
        aria-label="Workspace activity events"
        aria-live="off"
      >
        {query.isPending ? (
          <p className="p-4">Loading activity…</p>
        ) : items.length === 0 ? (
          <p className="p-4 text-sm text-muted-foreground">
            No matching activity recorded yet. Events appear as agents work;
            opening this view does not start a workspace.
          </p>
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="sticky top-0 bg-background">
              <tr>
                <th className="p-3">Time</th>
                <th className="p-3">Workspace / source</th>
                <th className="p-3">Activity</th>
              </tr>
            </thead>
            <tbody>
              {items.map((e) => (
                <tr key={e.workspace_id + e.id} className="border-t">
                  <td
                    className="p-3 whitespace-nowrap font-mono text-xs"
                    title={e.at}
                  >
                    {new Date(e.at).toLocaleTimeString()}
                  </td>
                  <td className="p-3">
                    <span>{names.get(e.workspace_id) || e.workspace_id}</span>
                    <span className="block text-xs text-muted-foreground">
                      {e.source}
                    </span>
                  </td>
                  <td className="p-3">
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
        )}
      </div>
    </Modal>
  );
}
