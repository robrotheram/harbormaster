import { request, requireProject, APIError } from "@/shared/http";
type Event = {
  id: string;
  at: string;
  source: string;
  kind: string;
  status?: string;
  tool?: string;
  task_id?: string;
  workspace_id: string;
  project_id: string;
};
export type Logs = {
  workspace_id?: string;
  project_id?: string;
  items: Event[];
  services?: { name: string; state: string }[];
  state?: string;
  active_runs?: number;
  background_jobs?: number;
};
export function checkedLogs(value: Logs, project: string, workspace?: string) {
  requireProject(value.items, project);
  if (
    workspace &&
    (value.workspace_id !== workspace ||
      value.project_id !== project ||
      value.items.some((e) => e.workspace_id !== workspace))
  )
    throw new APIError("NOT_FOUND_OR_FORBIDDEN", 404);
  return value;
}

export function logsOptions(project: string, workspace = "") {
  return {
    queryKey: ["project", project, "logs", workspace],
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      request<Logs>(
        workspace
          ? `/workspaces/${encodeURIComponent(workspace)}/logs`
          : `/workspace-logs?project_id=${encodeURIComponent(project)}`,
        undefined,
        signal,
      ).then((v) => checkedLogs(v, project, workspace || undefined)),
  };
}
