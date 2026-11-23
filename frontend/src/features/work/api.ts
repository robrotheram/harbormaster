import { request, projectList, requireProject, APIError } from "@/shared/http";

export type Artifact = {
  id?: string;
  source_path?: string;
  size_bytes?: number;
  sha256?: string;
  name?: string;
  kind?: string;
  content_url?: string;
};
export type WorkItem = {
  controller_id?: string;
  reviewer_id?: string;
  control_history?: { from: string; to: string; actor: string; at: string }[];
  provider?: string;
  model?: string;
  reasoning_effort?: string;
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
  reviewed_by?: string;
  id: string;
  work_item_id: string;
  workspace_id: string;
  task_id?: string;
  parent_attempt_id?: string;
  review_state?: string;
  review_feedback?: string;
  specification_path?: string;
  state: string;
  error_code?: string;
  artifacts?: Artifact[];
  created_at: string;
  updated_at: string;
};
export type TaskChanges = {
  files: { path: string; status: string; source?: string }[];
  diff?: string;
  truncated?: boolean;
};
export type WorkCreate = {
  provider?: string;
  model?: string;
  reasoning_effort?: string;
  project_id: string;
  workspace_id?: string;
  instruction?: string;
  specification_path?: string;
  specification_digest?: string;
  kind: WorkItem["kind"];
  artifact_paths?: string[];
};
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
  files: (project: string, id: string, signal: AbortSignal) =>
    request<{ paths: string[]; truncated?: boolean }>(
      path(id) + "/files" + scope(project),
      undefined,
      signal,
    ),
  file: (project: string, id: string, file: string, signal: AbortSignal) =>
    request<{ path: string; content?: string }>(
      path(id) + "/files" + scope(project),
      { path: file },
      signal,
    ),
  changes: (
    project: string,
    id: string,
    attempt: string,
    signal: AbortSignal,
  ) =>
    request<TaskChanges | null>(
      path(id) +
        "/attempts/" +
        encodeURIComponent(attempt) +
        "/changes" +
        scope(project),
      undefined,
      signal,
    ),
  instruction: async (project: string, id: string, signal?: AbortSignal) => {
    const value = await request<{
      project_id: string;
      work_item_id: string;
      instruction: string;
    }>(path(id) + "/instruction" + scope(project), undefined, signal);
    requireProject([value], project);
    if (value.work_item_id !== id)
      throw new APIError("NOT_FOUND_OR_FORBIDDEN", 403);
    return value.instruction;
  },
  begin: async (payload: WorkCreate, key: string) => {
    const item = await workApi.create(payload, key);
    try {
      await workApi.run(payload.project_id, item.id, key);
      return { item, error: undefined as unknown };
    } catch (error) {
      // Keep the saved draft reachable if dispatch could not be requested.
      return { item, error };
    }
  },
  continue: (
    project: string,
    id: string,
    instruction: string,
    expected_attempt_id: string,
    idempotency_key: string,
    selection?: { provider: string; model: string; reasoning_effort?: string },
    screenshots?: { content: string; note: string }[],
    workspace?: string,
  ) =>
    request<Attempt>(path(id) + "/continue", {
      project_id: project,
      instruction,
      presentation: "interactive-v1",
      ...(selection ? { selection } : {}),
      ...(screenshots?.length ? { screenshots } : {}),
      ...(workspace ? { workspace_id: workspace } : {}),
      expected_attempt_id,
      idempotency_key,
    }),
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
    request<WorkItem>("/work-items", {
      ...payload,
      presentation: "interactive-v1",
      idempotency_key,
    }),
  run: (
    project: string,
    id: string,
    idempotency_key: string,
    selection?: { provider: string; model: string; reasoning_effort?: string },
  ) =>
    request(path(id) + "/run", {
      project_id: project,
      idempotency_key,
      selection,
    }),
  cancel: (project: string, id: string) =>
    request(path(id) + "/cancel", { project_id: project }),
  review: (
    project: string,
    id: string,
    accepted: boolean,
    expected_attempt_id?: string,
    feedback?: string,
  ) =>
    request(path(id) + "/review", {
      project_id: project,
      accepted,
      expected_attempt_id,
      feedback,
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
    run: [
      "draft",
      "planned",
      "in_progress",
      "changes_requested",
      "failed",
      "cancelled",
      "rejected",
    ].includes(s),
    cancel: ["queued", "running", "blocked", "pending"].includes(s),
    review: [
      "awaiting_review",
      "review",
      "review_required",
      "pending_review",
    ].includes(s),
  };
}
