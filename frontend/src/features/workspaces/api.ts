import { request, projectList } from "@/shared/http";
import { keys } from "@/shared/query-keys";
import type {
  Workspace,
  StartResult,
  Pairing,
  AgentHome,
} from "@/shared/types";
export type WorkspaceCreate = {
  project_id: string;
  name: string;
  agent_kind: string;
  connection_profile_id?: string;
  parent_id?: string;
  branch?: string;
};
export type WorkspaceSettings = {
  idle_timeout_minutes: number;
  connection_profile_id?: string;
  connections?: import("@/shared/types").ConnectionPolicy;
};
export const workspacesApi = {
  delete: (id: string) =>
    request(`/workspaces/${encodeURIComponent(id)}/delete`, { confirm_id: id }),
  create: (payload: WorkspaceCreate, idempotency_key: string) =>
    request<Workspace>("/workspaces", { ...payload, idempotency_key }),
  settings: (id: string, payload: WorkspaceSettings) =>
    request<Workspace>(
      `/workspaces/${encodeURIComponent(id)}/settings`,
      payload,
    ),
  enableConsole: (id: string) =>
    request(`/workspaces/${encodeURIComponent(id)}/enable-t3`, {
      idempotency_key: crypto.randomUUID(),
    }),
  handoff: (
    source_id: string,
    source_path: string,
    destination_id: string,
    destination_path: string,
  ) =>
    request<{ bytes: number }>("/file-handoffs", {
      source_id,
      source_path,
      destination_id,
      destination_path,
    }),
  list: (project: string, signal?: AbortSignal) =>
    projectList<Workspace>("/workspaces", project, signal),
  homes: (project: string, signal?: AbortSignal) =>
    projectList<AgentHome>("/agent-homes", project, signal),
  start: (id: string, idempotency_key: string = crypto.randomUUID()) =>
    request<StartResult>(`/workspaces/${encodeURIComponent(id)}/start`, {
      idempotency_key,
    }),
  stop: (id: string) =>
    request<Workspace>(`/workspaces/${encodeURIComponent(id)}/stop`, {
      idempotency_key: crypto.randomUUID(),
    }),
  pair: (id: string) =>
    request<Pairing>(`/workspaces/${encodeURIComponent(id)}/desktop-pair`, {
      idempotency_key: crypto.randomUUID(),
    }),
};
export function workspaceOptions(project: string) {
  return {
    queryKey: keys.workspaces(project),
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      workspacesApi.list(project, signal),
    refetchInterval: 10000,
    retry: 1,
  };
}
