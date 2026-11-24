import { request, projectList } from "@/shared/http";
import { keys } from "@/shared/query-keys";
import type { Schedule, Run } from "@/shared/types";
export type ScheduleCreate = Pick<
  Schedule,
  | "project_id"
  | "workspace_id"
  | "name"
  | "instruction_path"
  | "provider"
  | "run_at"
  | "interval_seconds"
>;
export const schedulingApi = {
  create: (payload: ScheduleCreate, idempotency_key: string) =>
    request<Schedule>("/schedules", { ...payload, idempotency_key }),
  update: (
    id: string,
    payload: ScheduleCreate,
    revision: number,
    idempotency_key: string,
  ) =>
    request<Schedule>(`/schedules/${encodeURIComponent(id)}/update`, {
      ...payload,
      revision,
      idempotency_key,
    }),
  remove: (id: string, revision: number, idempotency_key: string) =>
    request(`/schedules/${encodeURIComponent(id)}/delete`, {
      revision,
      idempotency_key,
    }),
  schedules: (project: string, signal?: AbortSignal) =>
    projectList<Schedule>("/schedules", project, signal),
  runs: (project: string, signal?: AbortSignal) =>
    projectList<Run>("/runs", project, signal),
  action: (
    id: string,
    action: "run-now" | "enable" | "disable",
    idempotency_key: string,
  ) =>
    request(`/schedules/${encodeURIComponent(id)}/${action}`, {
      idempotency_key,
    }),
};
export const scheduleOptions = (project: string) => ({
  queryKey: keys.schedules(project),
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    schedulingApi.schedules(project, signal),
  refetchInterval: 10000,
});
export const runOptions = (project: string) => ({
  queryKey: keys.runs(project),
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    schedulingApi.runs(project, signal),
  refetchInterval: 10000,
});
