import { request } from "@/shared/http";
import { privateContentOptions } from "@/shared/private-content";
export interface Task {
  id: string;
  state: string;
  error_code?: string;
}
const path = (workspace: string, task?: string) =>
  `/workspaces/${encodeURIComponent(workspace)}/tasks${task ? `/${encodeURIComponent(task)}` : ""}`;
export const tasksApi = {
  submit: (
    workspace: string,
    instruction: string,
    provider: string,
    idempotency_key: string,
  ) =>
    request<Task>(path(workspace), { instruction, provider, idempotency_key }),
  cancel: (workspace: string, task: string) =>
    request<Task>(path(workspace, task) + "/cancel", {
      idempotency_key: crypto.randomUUID(),
    }),
};
export const taskKey = (project: string, workspace: string, task?: string) =>
  ["project", project, "task", workspace, task] as const;
export const taskOptions = (
  project: string,
  workspace: string,
  task: Task | undefined,
) => ({
  queryKey: taskKey(project, workspace, task?.id),
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    request<Task>(path(workspace, task!.id), undefined, signal),
  enabled: !!task,
  refetchInterval: (query: { state: { data: Task | undefined } }) =>
    ["queued", "running", "cancel_requested"].includes(
      query.state.data?.state || task?.state || "",
    )
      ? 3000
      : (false as const),
});
export const resultOptions = (
  project: string,
  workspace: string,
  task: string | undefined,
  enabled: boolean,
) => ({
  ...privateContentOptions,
  queryKey: [...taskKey(project, workspace, task), "result"],
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    request<{ content: string; next_offset: number }>(
      path(workspace, task) + "/result?limit=65536",
      undefined,
      signal,
    ),
  enabled,
});
