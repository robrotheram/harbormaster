import { APIError, request } from "@/shared/http";
import { checkedHistory, type Snapshot } from "@/features/history/history-api";
import type { Attempt } from "./api";

export async function runHistory(
  project: string,
  workspace: string,
  task: string,
  before = 0,
  signal?: AbortSignal,
  after?: number,
) {
  const value = checkedHistory(
    await request<Snapshot>(
      `/workspaces/${encodeURIComponent(workspace)}/history?episode=${encodeURIComponent(task)}${after !== undefined ? `&after=${after}` : `&order=desc${before ? `&before=${before}` : ""}`}`,
      undefined,
      signal,
    ),
    project,
    workspace,
  );
  if (value.page.records.some((r) => r.episode_id !== task))
    throw new APIError("NOT_FOUND_OR_FORBIDDEN", 403);
  return value;
}

// Fetch new events in ascending order so bursts larger than a single page do
// not silently drop activity. Keep events already shown while this view is open.
export async function refreshRunHistory(
  project: string,
  workspace: string,
  task: string,
  previous?: Snapshot,
  signal?: AbortSignal,
) {
  const after = previous
    ? previous.page.records.reduce((latest, r) => Math.max(latest, r.seq), 0)
    : undefined;
  const value = await runHistory(project, workspace, task, 0, signal, after);
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
  if (
    (attempt.parent_attempt_id &&
      content.startsWith("Continue the same work in this workspace.")) ||
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
