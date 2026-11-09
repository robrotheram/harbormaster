import { logsOptions } from "./logs-api";
import { useQuery } from "@tanstack/react-query";

import { State } from "@/shared/components";

import type { Workspace } from "@/shared/types";

type Service = { name: string; state: string };
const serviceStates: Record<string, string> = {
  connected: "Connected",
  starting: "Starting",
  failed: "Failed",
  disconnected: "Disconnected",
};
export function backgroundJobDetails(jobs: number, services: Service[]) {
  const known = services
    .filter(
      (s) =>
        s.name === "Matrix listener" && Object.hasOwn(serviceStates, s.state),
    )
    .slice(0, 1);
  const active = known.filter(
    (s) => s.state === "connected" || s.state === "starting",
  ).length;
  const remaining = Math.max(0, jobs - active);
  return {
    services: known.map((s) => `${s.name}: ${serviceStates[s.state]}`),
    remaining,
  };
}
export function WorkspaceActivity({
  workspace,
  onOpenActivity,
}: {
  workspace: Workspace;
  onOpenActivity: () => void;
}) {
  const hasJobs = workspace.ready && workspace.background_jobs > 0;
  const query = useQuery({
    ...logsOptions(workspace.project_id, workspace.id),
    enabled: hasJobs && workspace.agent_kind === "hermes",
    refetchInterval: hasJobs ? 5000 : false,
  });
  const details = backgroundJobDetails(
    workspace.background_jobs,
    hasJobs && !query.isError ? query.data?.services || [] : [],
  );
  const count =
    workspace.state === "STOPPED"
      ? "Files retained"
      : workspace.active_runs || workspace.background_jobs
        ? `${workspace.active_runs} ${workspace.active_runs === 1 ? "run" : "runs"} / ${workspace.background_jobs} ${workspace.background_jobs === 1 ? "job" : "jobs"}`
        : workspace.state === "READY"
          ? "Idle"
          : "Checking activity";
  return (
    <>
      <State value={workspace.state} />
      <div className="mt-2 text-xs text-muted-foreground">{count}</div>
      {hasJobs && (
        <button
          type="button"
          onClick={onOpenActivity}
          aria-label={`View background job activity for ${workspace.name || "workspace"}: ${details.services.join(", ") || `${workspace.background_jobs} background processes`}`}
          className="mt-1.5 block text-left text-xs text-muted-foreground underline decoration-dotted underline-offset-4 hover:text-foreground focus-visible:rounded focus-visible:outline-2 focus-visible:outline-ring max-md:ml-auto max-md:text-right"
        >
          {details.services.map((text) => (
            <span key={text} className="block">
              {text}
            </span>
          ))}
          {details.remaining > 0 && (
            <span className="block">
              {query.isPending && workspace.agent_kind === "hermes"
                ? "Checking background jobs…"
                : query.isError
                  ? "Job details unavailable — view activity"
                  : `${details.remaining} ${details.services.length ? "other " : ""}background ${details.remaining === 1 ? "process" : "processes"} — view activity`}
            </span>
          )}
        </button>
      )}
    </>
  );
}
