import { afterEach, describe, expect, it, vi } from "vitest";
import { runHistory, refreshRunHistory, userMessage } from "./conversation-api";
import type { Attempt } from "./api";
import type { Snapshot, HistoryRecord } from "@/features/history/history-api";

afterEach(() => vi.unstubAllGlobals());
describe("work conversation boundaries", () => {
  it("requests one task and rejects foreign project, worker, or task records", async () => {
    const snapshot = {
      project_id: "project_one",
      workspace_id: "ws_one",
      page: { schema_version: 1, records: [{ episode_id: "task_one" }] },
    };
    const fetcher = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify(snapshot)),
    );
    vi.stubGlobal("fetch", fetcher);
    await expect(
      runHistory("project_one", "ws_one", "task_one", 7),
    ).resolves.toEqual(snapshot);
    expect(String(fetcher.mock.calls[0][0])).toContain(
      "episode=task_one&order=desc&before=7",
    );
    await expect(
      runHistory("project_other", "ws_one", "task_one"),
    ).rejects.toThrow();
    await expect(
      runHistory("project_one", "ws_other", "task_one"),
    ).rejects.toThrow();
    snapshot.page.records.push({ episode_id: "task_other" });
    await expect(
      runHistory("project_one", "ws_one", "task_one"),
    ).rejects.toThrow();
  });
  it("retains displayed events and catches up on bursts without skipping pages", async () => {
    const records = Array.from(
      { length: 62 },
      (_, i) =>
        ({
          id: `r${i + 1}`,
          seq: i + 1,
          episode_id: "task_one",
        }) as HistoryRecord,
    );
    const previous = {
      project_id: "project_one",
      workspace_id: "ws_one",
      page: {
        records: records.slice(20, 40),
        next_before: 21,
        high_watermark: 40,
        schema_version: 1,
        capture_limited: false,
        bytes_used: 0,
      },
    } as Snapshot;
    const seen: number[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const after = Number(
          new URL(String(input), "http://localhost").searchParams.get("after"),
        );
        seen.push(after);
        const page = records.filter((r) => r.seq > after).slice(0, 20);
        return new Response(
          JSON.stringify({
            ...previous,
            page: {
              ...previous.page,
              records: page,
              next_before: 0,
              next_after: page.length === 20 ? page.at(-1)!.seq : 0,
            },
          }),
        );
      }),
    );
    const first = await refreshRunHistory(
      "project_one",
      "ws_one",
      "task_one",
      previous,
    );
    expect(first.page.next_after).toBe(60);
    const second = await refreshRunHistory(
      "project_one",
      "ws_one",
      "task_one",
      first,
    );
    expect(seen).toEqual([40, 60]);
    expect(second.page.records.map((r) => r.seq)).toEqual(
      records.slice(20).map((r) => r.seq),
    );
    expect(second.page.next_before).toBe(21);
    expect(second.page.next_after).toBe(0);
  });
  it("hides the presentation protocol on initial requests", () => {
    expect(
      userMessage(
        "Workbench presentation: interactive-v1.\nChart instructions.\n\nCurrent request:\nExplore disk usage.",
        {},
      ),
    ).toBe("Explore disk usage.");
  });
  it("shows the original user text unless this is a generated follow-up instruction", () => {
    const message =
      "Continue the same work in this workspace. Read the previous request.\n\nCurrent request:\nAdd tests.\n\nCurrent request:\nKeep this literal heading.";
    expect(userMessage(message, {} as Attempt)).toBe(message);
    expect(
      userMessage(message, { parent_attempt_id: "attempt_one" } as Attempt),
    ).toBe("Add tests.\n\nCurrent request:\nKeep this literal heading.");
    const withContext =
      'Before acting, read "work-requests/context-task_one-123.md" and follow its earlier context references.\n\n' +
      message;
    expect(userMessage(withContext, { parent_attempt_id: "attempt_one" })).toBe(
      "Add tests.\n\nCurrent request:\nKeep this literal heading.",
    );
    expect(
      userMessage("My own prompt\n\nCurrent request:\nLeave intact.", {
        parent_attempt_id: "attempt_one",
      } as Attempt),
    ).toBe("My own prompt\n\nCurrent request:\nLeave intact.");
  });
});
