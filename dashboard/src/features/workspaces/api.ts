import {
  retryTransient,
  pollUnlessBlocked,
  refocusUnlessBlocked,
} from "@/shared/query-policy";
import { request, requireProject } from "@/shared/http";
import { keys } from "@/shared/query-keys";
import type { Workspace, StartResult, Pairing } from "@/shared/types";
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
export interface StartReadiness {
  workspace_id: string;
  project_id: string;
  checked_at: string;
  can_start: boolean;
  checks: {
    id: string;
    label: string;
    status: "passed" | "blocked" | "unknown";
    code?: string;
    message: string;
  }[];
}
export const workspacesApi = {
  readiness: async (workspace: Workspace, signal?: AbortSignal) => {
    const result = await request<StartReadiness>(
      `/workspaces/${encodeURIComponent(workspace.id)}/start-readiness`,
      undefined,
      signal,
    );
    requireProject([result], workspace.project_id);
    if (result.workspace_id !== workspace.id)
      throw new Error(
        "The startup check returned a different worker. Refresh and try again.",
      );
    return result;
  },
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
  list: async (project: string, signal?: AbortSignal) => {
    const value = await request<{ items: Workspace[] }>(
      "/workspaces?project_id=" + encodeURIComponent(project),
      undefined,
      signal,
    );
    requireProject(
      value.items.filter((workspace) => workspace.shared !== true),
      project,
    );
    return value.items;
  },
  start: (id: string, idempotency_key: string = crypto.randomUUID()) =>
    request<StartResult>(`/workspaces/${encodeURIComponent(id)}/start`, {
      idempotency_key,
    }),
  stop: (id: string, idempotency_key: string = crypto.randomUUID()) =>
    request<Workspace>(`/workspaces/${encodeURIComponent(id)}/stop`, {
      idempotency_key,
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
    refetchInterval: pollUnlessBlocked(10000),
    retry: retryTransient,
    retryOnMount: false,
    refetchOnWindowFocus: refocusUnlessBlocked,
    refetchOnReconnect: refocusUnlessBlocked,
    refetchOnMount: refocusUnlessBlocked,
  };
}
