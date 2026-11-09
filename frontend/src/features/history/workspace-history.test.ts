import { describe, it, expect } from "vitest";
import {
  checkedHistory,
  conversationMessages,
  loadConversationPage,
  type HistoryRecord,
} from "@/features/history/history-api";
describe("private workspace history", () => {
  it("rejects another project's or workspace's content and unknown schemas", () => {
    const snapshot = {
      workspace_id: "ws-one",
      project_id: "project-one",
      page: {
        records: [],
        next_before: 0,
        high_watermark: 0,
        bytes_used: 0,
        capture_limited: false,
        schema_version: 1,
      },
      ratings: {},
    };
    expect(checkedHistory(snapshot, "project-one", "ws-one")).toBe(snapshot);
    expect(() => checkedHistory(snapshot, "project-two", "ws-one")).toThrow();
    expect(() => checkedHistory(snapshot, "project-one", "ws-two")).toThrow();
    expect(() =>
      checkedHistory(
        { ...snapshot, page: { ...snapshot.page, schema_version: 2 } },
        "project-one",
        "ws-one",
      ),
    ).toThrow();
  });
});

function record(seq: number, kind: string, content = ""): HistoryRecord {
  return {
    seq,
    id: `hist_${seq}`,
    at: "2026-11-08T00:00:00Z",
    captured_at: "2026-11-08T00:00:00Z",
    episode_id: "episode-one",
    source: "matrix",
    kind,
    content,
    capture_scope: "live",
  };
}
function page(records: HistoryRecord[], next_before = 0) {
  return {
    workspace_id: "ws-one",
    project_id: "project-one",
    page: {
      records,
      next_before,
      high_watermark: 100,
      bytes_used: 1000,
      capture_limited: false,
      schema_version: 1,
    },
  };
}
describe("conversation history", () => {
  it("shows messages and replies once, excluding tools, lifecycle and delivery echoes", () => {
    const messages = conversationMessages([
      record(6, "message.delivery", "Hello!"),
      record(5, "run.completed"),
      record(4, "assistant.message", "Hello!"),
      record(3, "tool.result", "output"),
      record(2, "run.started"),
      record(1, "user.message", "Hello"),
      record(7, "assistant.message", "   "),
    ]);
    expect(messages.map((m) => m.content)).toEqual(["Hello", "Hello!"]);
  });
  it("reads through event-only pages to find the conversation", async () => {
    const cursors: number[] = [];
    const result = await loadConversationPage(
      "project-one",
      "ws-one",
      0,
      async (before) => {
        cursors.push(before);
        return before === 0
          ? page(
              [
                record(25, "tool.result", "output"),
                record(24, "message.delivery", "Hello!"),
              ],
              20,
            )
          : page([
              record(19, "assistant.message", "Hello!"),
              record(18, "user.message", "Hello"),
            ]);
      },
    );
    expect(cursors).toEqual([0, 20]);
    expect(result.page.records.map((m) => m.content)).toEqual([
      "Hello",
      "Hello!",
    ]);
    expect(result.page.next_before).toBe(0);
  });
  it("checks project ownership on every page", async () => {
    await expect(
      loadConversationPage("project-one", "ws-one", 0, async (before) =>
        before === 0
          ? page([record(25, "run.started")], 20)
          : {
              ...page([record(19, "user.message", "foreign")]),
              project_id: "project-two",
            },
      ),
    ).rejects.toThrow();
  });
  it("bounds scans during long tool runs and retains the older cursor", async () => {
    let calls = 0;
    const result = await loadConversationPage(
      "project-one",
      "ws-one",
      0,
      async () => {
        calls++;
        return page(
          [record(100 - calls, "tool.result", "output")],
          100 - calls,
        );
      },
    );
    expect(calls).toBe(10);
    expect(result.page.records).toEqual([]);
    expect(result.page.next_before).toBe(90);
  });
  it("rejects a repeated paging cursor", async () => {
    await expect(
      loadConversationPage("project-one", "ws-one", 20, async () =>
        page([record(19, "run.started")], 20),
      ),
    ).rejects.toThrow();
  });
});
