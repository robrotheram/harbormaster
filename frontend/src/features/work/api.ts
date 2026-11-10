import { request, projectList, requireProject, APIError } from "@/shared/http";

export type Artifact = {
  id?: string;
  name?: string;
  kind?: string;
  content_url?: string;
};
export type WorkItem = {
  id: string;
  project_id: string;
  workspace_id: string;
  specification_path: string;
  specification_digest: string;
  kind: "software" | "research" | "digest";
  state: string;
  artifact_paths?: string[];
  latest_attempt_id?: string;
  error_code?: string;
  created_at: string;
  updated_at: string;
};
export type Attempt = {
  id: string;
  work_item_id: string;
  workspace_id: string;
  task_id?: string;
  state: string;
  error_code?: string;
  artifacts?: Artifact[];
  created_at: string;
  updated_at: string;
};
export type WorkCreate = Pick<
  WorkItem,
  | "project_id"
  | "workspace_id"
  | "specification_path"
  | "specification_digest"
  | "kind"
> & { artifact_paths?: string[] };
const path = (id: string) => "/work-items/" + encodeURIComponent(id);
const scope = (project: string) => "?project_id=" + encodeURIComponent(project);
export const workKeys = {
  list: (project: string) => ["project", project, "work"] as const,
  detail: (project: string, id: string) =>
    ["project", project, "work", id] as const,
  attempts: (project: string, id: string) =>
    ["project", project, "work", id, "attempts"] as const,
};
export const workApi = {
  list: (project: string, signal?: AbortSignal) =>
    projectList<WorkItem>("/work-items", project, signal),
  detail: async (project: string, id: string, signal?: AbortSignal) => {
    const item = await request<WorkItem>(
      path(id) + scope(project),
      undefined,
      signal,
    );
    requireProject([item], project);
    if (item.id !== id) throw new APIError("NOT_FOUND_OR_FORBIDDEN", 403);
    return item;
  },
  attempts: async (project: string, id: string, signal?: AbortSignal) => {
    const value = await request<{ items: Attempt[] }>(
      path(id) + "/attempts" + scope(project),
      undefined,
      signal,
    );
    if (value.items.some((a) => a.work_item_id !== id))
      throw new APIError("NOT_FOUND_OR_FORBIDDEN", 403);
    return value.items;
  },
  create: (payload: WorkCreate, idempotency_key: string) =>
    request<WorkItem>("/work-items", { ...payload, idempotency_key }),
  run: (project: string, id: string, idempotency_key: string) =>
    request(path(id) + "/run", { project_id: project, idempotency_key }),
  cancel: (project: string, id: string) =>
    request(path(id) + "/cancel", { project_id: project }),
  review: (
    project: string,
    id: string,
    accepted: boolean,
    expected_attempt_id?: string,
  ) =>
    request(path(id) + "/review", {
      project_id: project,
      accepted,
      expected_attempt_id,
    }),
};
export const workOptions = (project: string) => ({
  queryKey: workKeys.list(project),
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    workApi.list(project, signal),
  refetchInterval: 5000,
});
// Only backend-returned HTTP content links or existing same-origin API paths.
export function evidenceURL(value?: string) {
  if (!value || value.trim() !== value) return undefined;
  if (/^\/(?:v1|ui-api|dashboard-api)\//.test(value) && !value.includes("\\"))
    return value.startsWith("/v1/") ? "/ui-api/" + value.slice(4) : value;
  try {
    const url = new URL(value);
    return url.protocol === "https:" || url.protocol === "http:"
      ? url.href
      : undefined;
  } catch {
    return undefined;
  }
}
export function workActions(state: string) {
  const s = state.toLowerCase();
  return {
    run: ["draft", "failed", "cancelled", "rejected"].includes(s),
    cancel: ["queued", "running", "blocked", "pending"].includes(s),
    review: [
      "awaiting_review",
      "review",
      "review_required",
      "pending_review",
      "completed",
    ].includes(s),
  };
}
