import { APIError, request } from "@/shared/http";
import { checkedHistory, type Snapshot } from "@/features/history/history-api";
import type { Attempt } from "./api";

type Run = Pick<Attempt, "id" | "work_item_id" | "workspace_id" | "task_id">;

export async function runHistory(
  project: string,
  attempt: Run,
  before = 0,
  signal?: AbortSignal,
  after?: number,
) {
  const value = checkedHistory(
    await request<Snapshot>(
      `/work-items/${encodeURIComponent(attempt.work_item_id)}/attempts/${encodeURIComponent(attempt.id)}/history?project_id=${encodeURIComponent(project)}${after !== undefined ? `&after=${after}` : `&order=desc${before ? `&before=${before}` : ""}`}`,
      undefined,
      signal,
    ),
    project,
    attempt.workspace_id,
  );
  if (value.page.records.some((r) => r.episode_id !== attempt.task_id))
    throw new APIError("NOT_FOUND_OR_FORBIDDEN", 403);
  return value;
}

// Fetch new events in ascending order so bursts larger than a single page do
// not silently drop activity. Keep events already shown while this view is open.
export async function refreshRunHistory(
  project: string,
  attempt: Run,
  previous?: Snapshot,
  signal?: AbortSignal,
) {
  const after = previous
    ? previous.page.records.reduce((latest, r) => Math.max(latest, r.seq), 0)
    : undefined;
  const value = await runHistory(project, attempt, 0, signal, after);
  if (!previous) return value;
  return {
    ...value,
    page: {
      ...value.page,
      next_before: previous.page.next_before,
      records: [
        ...new Map(
          [...previous.page.records, ...value.page.records].map((r) => [
            r.id,
            r,
          ]),
        ).values(),
      ],
    },
  };
}

export function userMessage(
  content: string,
  attempt: Pick<Attempt, "parent_attempt_id">,
) {
  // Follow-up requests have a platform-generated file-context preamble. Keep
  // that in the stored instruction, but show the user's message in the thread.
  const marker = "\n\nCurrent request:\n";
  const followup = "Continue the same work in this workspace.";
  const hasContextPreamble =
    content.startsWith('Before acting, read "work-requests/context-') &&
    content.split("\n\n", 2)[1]?.startsWith(followup);
  if (
    (attempt.parent_attempt_id &&
      (content.startsWith(followup) ||
        content.startsWith(
          'Continue this conversation on this worker. Read "work-requests/context-',
        ) ||
        hasContextPreamble)) ||
    content.startsWith("Workbench presentation: interactive-v1.\n")
  ) {
    const at = content.indexOf(marker);
    if (at >= 0) return content.slice(at + marker.length);
  }
  return content;
}

export function activeAttempt(state: string) {
  return [
    "queued",
    "running",
    "dispatching",
    "blocked",
    "cancel_requested",
  ].includes(state);
}
