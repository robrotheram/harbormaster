import { checkedLogs } from "./logs-api";
import { describe, it, expect } from "vitest";
import { eventText } from "@/features/history/workspace-logs";
describe("workspace activity logs", () => {
  it("rejects foreign workspace and project events", () => {
    const value = {
      workspace_id: "one",
      project_id: "project-a",
      items: [
        {
          id: "event",
          at: "2026-11-08T00:00:00Z",
          source: "matrix",
          kind: "service.connected",
          workspace_id: "one",
          project_id: "project-a",
        },
      ],
    };
    expect(() => checkedLogs(value, "project-b")).toThrow();
    expect(() => checkedLogs(value, "project-a", "other")).toThrow();
    expect(checkedLogs(value, "project-a", "one").items).toHaveLength(1);
  });
  it("rejects a foreign service snapshot even when there are no events", () => {
    const value = {
      items: [],
      workspace_id: "other",
      project_id: "project-a",
      services: [{ name: "Matrix listener", state: "connected" }],
    };
    expect(() => checkedLogs(value, "project-a", "one")).toThrow();
    expect(() =>
      checkedLogs(
        { ...value, workspace_id: "one", project_id: "project-b" },
        "project-a",
        "one",
      ),
    ).toThrow();
  });
  it("explains listener and tool activity", () => {
    expect(eventText({ kind: "service.connected" })).toBe("Listener connected");
    expect(eventText({ kind: "tool.started", tool: "terminal" })).toBe(
      "Tool started · terminal",
    );
  });
});
