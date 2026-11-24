import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice } from "@/shared/components";
import { APIError, request } from "@/shared/http";
import { privateContentOptions } from "@/shared/private-content";
import { workspaceOptions } from "@/features/workspaces/api";
import type { Project } from "@/shared/types";
import type { HistoryRecord } from "./history-api";

type AuditPage = {
  project_id: string;
  next_before: number;
  items: { cursor: number; workspace_id: string; record: HistoryRecord }[];
};

const eventTypes: Record<string, string> = {
  "user.message": "User message",
  "assistant.message": "Agent reply",
  "tool.call": "Tool call",
  "tool.result": "Tool result",
  "run.started": "Run started",
  "run.completed": "Run completed",
  "run.failed": "Run failed",
  "run.cancelled": "Run cancelled",
  "run.usage": "Model usage",
  "capture.gap": "Recording gap",
  "message.delivery": "Message delivery",
};

export function ProjectAudit({ project }: { project: Project }) {
  const { data: workers = [] } = useQuery(workspaceOptions(project.id));
  const [worker, setWorker] = useState("");
  const [kind, setKind] = useState("");
  const [before, setBefore] = useState(0);
  const query = useQuery({
    ...privateContentOptions,
    queryKey: ["project", project.id, "audit-history", worker, kind, before],
    queryFn: async ({ signal }) => {
      const params = new URLSearchParams({ workspace_id: worker, kind });
      if (before) params.set("before", String(before));
      const data = await request<AuditPage>(
        `/projects/${encodeURIComponent(project.id)}/audit-history?${params}`,
        undefined,
        signal,
      );
      if (
        data.project_id !== project.id ||
        data.items.some(
          (item) =>
            (worker && item.workspace_id !== worker) ||
            (kind && item.record.kind !== kind),
        )
      )
        throw new APIError("NOT_FOUND_OR_FORBIDDEN", 404);
      return data;
    },
    refetchInterval: before ? false : 5000,
  });
  const next = query.data?.next_before;
  return (
    <section aria-labelledby="project-audit-history" className="settings-panel">
      <h2 id="project-audit-history" className="settings-panel-title">
        Audit history
      </h2>
      <div className="flex flex-wrap items-center gap-2">
        <NativeSelect
          aria-label="Audit worker"
          value={worker}
          onChange={(event) => {
            setWorker(event.target.value);
            setBefore(0);
          }}
        >
          <option value="">All workers</option>
          {workers.map((worker) => (
            <option key={worker.id} value={worker.id}>
              {worker.name || worker.id}
            </option>
          ))}
        </NativeSelect>
        <NativeSelect
          aria-label="Audit event type"
          value={kind}
          onChange={(event) => {
            setKind(event.target.value);
            setBefore(0);
          }}
        >
          <option value="">All activity</option>
          {Object.entries(eventTypes).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </NativeSelect>
        <Button
          variant="outline"
          onClick={() => (before ? setBefore(0) : void query.refetch())}
        >
          Newest
        </Button>
        <Button
          variant="outline"
          disabled={!next}
          onClick={() => next && setBefore(next)}
        >
          Older
        </Button>
      </div>
      {query.isError && (
        <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />
      )}
      {query.isPending && <p role="status">Loading audit history…</p>}
      {query.data?.items.length === 0 && (
        <p className="py-4 text-sm text-muted-foreground">
          No cached events match these filters. History syncs while workers are
          online.
        </p>
      )}
      <ol className="divide-y" aria-label="Audit events">
        {query.data?.items.map(({ cursor, workspace_id, record }) => {
          const name = workers.find(
            (worker) => worker.id === workspace_id,
          )?.name;
          return (
            <li key={cursor}>
              <details className="px-1 py-2">
                <summary className="cursor-pointer break-words text-sm">
                  <span className="font-medium">
                    {eventTypes[record.kind] || record.kind}
                  </span>
                  {record.tool && (
                    <span className="ml-2 font-mono text-xs">
                      {[record.tool_server, record.tool]
                        .filter(Boolean)
                        .join(" / ")}
                    </span>
                  )}
                  <span className="ml-2 text-xs text-muted-foreground">
                    {name || workspace_id}{" "}
                    <time dateTime={record.at}>
                      {new Date(record.at).toLocaleString()}
                    </time>
                  </span>
                </summary>
                <div className="space-y-2 py-2 pl-4 text-sm">
                  {record.content && (
                    <pre className="whitespace-pre-wrap break-words rounded-md bg-muted p-3 text-xs [overflow-wrap:anywhere]">
                      {record.content}
                    </pre>
                  )}
                  <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 break-words text-xs text-muted-foreground">
                    <dt>Worker ID</dt>
                    <dd>{workspace_id}</dd>
                    <dt>Episode ID</dt>
                    <dd>{record.episode_id}</dd>
                    <dt>Source</dt>
                    <dd>
                      {[record.source, record.provider, record.model]
                        .filter(Boolean)
                        .join(" / ")}
                    </dd>
                    <dt>Captured</dt>
                    <dd>{new Date(record.captured_at).toLocaleString()}</dd>
                    {record.tool_server && (
                      <>
                        <dt>MCP server</dt>
                        <dd>{record.tool_server}</dd>
                      </>
                    )}
                    {record.tool_call_id && (
                      <>
                        <dt>Call ID</dt>
                        <dd>{record.tool_call_id}</dd>
                      </>
                    )}
                    {record.status && (
                      <>
                        <dt>Status</dt>
                        <dd>{record.status}</dd>
                      </>
                    )}
                    {record.purpose === "test" && (
                      <>
                        <dt>Purpose</dt>
                        <dd>Test run</dd>
                      </>
                    )}
                    {(record.redacted ||
                      record.truncated ||
                      record.backfilled) && (
                      <>
                        <dt>Recording</dt>
                        <dd>
                          {[
                            record.redacted && "Redacted",
                            record.truncated && "Shortened",
                            record.backfilled && "Imported",
                          ]
                            .filter(Boolean)
                            .join(", ")}
                        </dd>
                      </>
                    )}
                  </dl>
                </div>
              </details>
            </li>
          );
        })}
      </ol>
    </section>
  );
}
