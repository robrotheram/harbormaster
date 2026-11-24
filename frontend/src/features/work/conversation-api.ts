import { APIError, request } from "@/shared/http";
import { checkedHistory, type Snapshot } from "@/features/history/history-api";
import type { Attempt } from "./api";

type Run = Pick<Attempt, "id" | "work_item_id" | "workspace_id" | "task_id">;

function historyPath(attempt: Run) {
  return `/work-items/${encodeURIComponent(attempt.work_item_id)}/attempts/${encodeURIComponent(attempt.id)}/history`;
}

function checkedRunHistory(value: Snapshot, project: string, attempt: Run) {
  checkedHistory(value, project, attempt.workspace_id);
  if (value.page.records.some((r) => r.episode_id !== attempt.task_id))
    throw new APIError("NOT_FOUND_OR_FORBIDDEN", 403);
  return value;
}

export async function runHistory(
  project: string,
  attempt: Run,
  before = 0,
  signal?: AbortSignal,
  after?: number,
) {
  return checkedRunHistory(
    await request<Snapshot>(
      `${historyPath(attempt)}?project_id=${encodeURIComponent(project)}${after !== undefined ? `&after=${after}` : `&order=desc${before ? `&before=${before}` : ""}`}`,
      undefined,
      signal,
    ),
    project,
    attempt,
  );
}

export function historyAfter(snapshot?: Snapshot) {
  return snapshot?.page.records.reduce(
    (latest, r) => Math.max(latest, r.seq),
    0,
  );
}

export function watchRunHistory(
  project: string,
  attempt: Run,
  after: number,
  onLive: (live: boolean) => void,
  onHistory: (snapshot: Snapshot) => void,
) {
  const url = `/ui-api${historyPath(attempt)}/events?project_id=${encodeURIComponent(project)}&after=${after}`;
  const source = new EventSource(url);
  const stop = () => {
    source.close();
    onLive(false);
  };
  source.onopen = () => onLive(true);
  source.onerror = () => onLive(false);
  source.addEventListener("done", stop);
  source.addEventListener("unavailable", stop);
  source.addEventListener("history", (event: MessageEvent<string>) => {
    let value: Snapshot;
    try {
      value = checkedRunHistory(JSON.parse(event.data), project, attempt);
    } catch {
      stop();
      return;
    }
    onHistory(value);
  });
  return () => source.close();
}

// Fetch new events in ascending order so bursts larger than a single page do
// not silently drop activity. Keep events already shown while this view is open.
export async function refreshRunHistory(
  project: string,
  attempt: Run,
  previous?: Snapshot,
  signal?: AbortSignal,
) {
  const value = await runHistory(
    project,
    attempt,
    0,
    signal,
    historyAfter(previous),
  );
  return mergeRunHistory(previous, value);
}

export function mergeRunHistory(
  previous: Snapshot | undefined,
  value: Snapshot,
) {
  if (!previous) return value;
  return {
    ...value,
    page: {
      ...value.page,
      next_before: previous.page.next_before,
      high_watermark: Math.max(
        previous.page.high_watermark,
        value.page.high_watermark,
      ),
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
