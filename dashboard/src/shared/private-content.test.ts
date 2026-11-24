import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { privateContentOptions } from "./private-content";
describe("private transcript and result cache lifetime", () => {
  it("drops private content after its final view closes while retaining unrelated metadata", async () => {
    vi.useFakeTimers();
    const client = new QueryClient();
    try {
      const key = ["project", "private-project", "history", "workspace", 0];
      client.setQueryData(
        ["project", "private-project", "workspaces"],
        [{ id: "workspace" }],
      );
      const observer = new QueryObserver(client, {
        ...privateContentOptions,
        queryKey: key,
        queryFn: async () => "private transcript",
      });
      const unsubscribe = observer.subscribe(() => {});
      await observer.refetch();
      expect(client.getQueryData(key)).toBe("private transcript");
      unsubscribe();
      await vi.runOnlyPendingTimersAsync();
      expect(client.getQueryData(key)).toBeUndefined();
      expect(
        client.getQueryData(["project", "private-project", "workspaces"]),
      ).toEqual([{ id: "workspace" }]);
    } finally {
      client.clear();
      vi.useRealTimers();
    }
  });
});
