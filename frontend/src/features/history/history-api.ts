import { request, APIError } from "@/shared/http";
import { privateContentOptions } from "@/shared/private-content";
export type HistoryRecord = {
  purpose?: "test" | "normal";
  seq: number;
  id: string;
  at: string;
  captured_at: string;
  episode_id: string;
  session_id?: string;
  source: string;
  kind: string;
  role?: string;
  content?: string;
  tool?: string;
  model?: string;
  provider?: string;
  channel?: string;
  status?: string;
  redacted?: boolean;
  truncated?: boolean;
  backfilled?: boolean;
  capture_scope: string;
  input_tokens?: number;
  output_tokens?: number;
  usage_scope?: string;
  elapsed_ms?: number;
};
export type Snapshot = {
  workspace_id: string;
  project_id: string;
  page: {
    records: HistoryRecord[];
    next_before: number;
    high_watermark: number;
    bytes_used: number;
    capture_limited: boolean;
    schema_version: number;
  };
};
export function checkedHistory(
  value: Snapshot,
  project: string,
  workspace: string,
) {
  if (
    value.project_id !== project ||
    value.workspace_id !== workspace ||
    value.page.schema_version !== 1
  )
    throw new APIError("NOT_FOUND_OR_FORBIDDEN", 404);
  return value;
}
export function conversationMessages(records: HistoryRecord[]) {
  return records
    .filter(
      (r) =>
        (r.kind === "user.message" || r.kind === "assistant.message") &&
        r.content?.trim(),
    )
    .sort((a, b) => a.seq - b.seq);
}
// Read across raw event pages so tools and delivery receipts do not crowd out
// the conversation. Bound each request to keep long tool-only runs responsive.
export async function loadConversationPage(
  project: string,
  workspace: string,
  before: number,
  read: (before: number) => Promise<Snapshot>,
) {
  let snapshot: Snapshot | undefined;
  const records: HistoryRecord[] = [];
  let cursor = before;
  for (let page = 0; page < 10; page++) {
    snapshot = checkedHistory(await read(cursor), project, workspace);
    records.push(...snapshot.page.records);
    const next = snapshot.page.next_before;
    if (
      !next ||
      records.filter(
        (r) =>
          (r.kind === "user.message" || r.kind === "assistant.message") &&
          r.content?.trim(),
      ).length >= 20
    )
      break;
    if (next === cursor || (cursor > 0 && next >= cursor))
      throw new APIError("HISTORY_UNAVAILABLE", 503);
    cursor = next;
  }
  return {
    ...snapshot!,
    page: { ...snapshot!.page, records: conversationMessages(records) },
  };
}

export function historyOptions(
  project: string,
  workspace: string,
  before: number,
) {
  return {
    ...privateContentOptions,
    queryKey: ["project", project, "history", workspace, before],
    enabled: !!workspace,
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      loadConversationPage(project, workspace, before, (cursor) =>
        request<Snapshot>(
          `/workspaces/${encodeURIComponent(workspace)}/history?order=desc${cursor ? `&before=${cursor}` : ""}`,
          undefined,
          signal,
        ),
      ),
    refetchInterval: before ? (false as const) : 5000,
  };
}
