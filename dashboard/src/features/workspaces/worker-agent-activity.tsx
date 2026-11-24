import { useQuery } from "@tanstack/react-query";
import { eventText } from "@/features/history/workspace-logs";
import { logsOptions } from "@/features/history/logs-api";
import { date } from "@/shared/format";
import type { Workspace } from "@/shared/types";

const agentNames: Record<string, string> = {
  t3: "T3 / Codex",
  codex: "Codex",
  claude: "Claude",
  hermes: "Hermes",
  matrix: "Hermes via Matrix",
};

export function WorkerAgentActivity({
  projectID,
  worker,
  watchingHistory,
}: {
  projectID: string;
  worker: Workspace;
  watchingHistory: boolean;
}) {
  const activity = useQuery({
    ...logsOptions(projectID, worker.id),
    refetchInterval: watchingHistory ? 5000 : false,
  });
  const latest = activity.data?.items
    .filter((event) => Object.hasOwn(agentNames, event.source))
    .reduce<(typeof activity.data.items)[number] | undefined>(
      (previous, event) =>
        !previous || Date.parse(event.at) > Date.parse(previous.at)
          ? event
          : previous,
      undefined,
    );

  const observed = activity.isError ? undefined : activity.data;
  return (
    <section
      aria-labelledby="agent-activity-title"
      className="space-y-3 rounded-md border bg-card p-4"
    >
      <h2 id="agent-activity-title" className="text-sm font-semibold">
        Agent activity
      </h2>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm md:grid-cols-4">
        <div>
          <dt className="text-xs text-muted-foreground">Configured agent</dt>
          <dd className="mt-1 font-medium">
            {agentNames[worker.agent_kind ?? ""] ?? "Unknown"}
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Active runs</dt>
          <dd className="mt-1 font-medium tabular-nums">
            {observed?.active_runs ?? worker.active_runs}
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Background jobs</dt>
          <dd className="mt-1 font-medium tabular-nums">
            {observed?.background_jobs ?? worker.background_jobs}
          </dd>
        </div>
        <div className="min-w-0">
          <dt className="text-xs text-muted-foreground">
            Latest recorded agent event
          </dt>
          <dd className="mt-1 break-words font-medium">
            {activity.isError
              ? "Activity unavailable"
              : activity.isPending
                ? "Checking activity…"
                : latest
                  ? `${agentNames[latest.source]} · ${eventText(latest)}`
                  : "No agent events yet"}
          </dd>
          {latest && !activity.isError && (
            <dd className="mt-1 text-xs text-muted-foreground">
              <time dateTime={latest.at}>{date(latest.at)}</time>
            </dd>
          )}
        </div>
      </dl>
    </section>
  );
}
