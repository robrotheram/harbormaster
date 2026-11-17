import { expect, it } from "vitest";
import { teamScope, teamActivity, teamAttention } from "./team-state";
import type { BoardCard } from "./board-api";
const card = (state: string) =>
  ({ work: { state }, stage: "planned" }) as BoardCard;
it("separates planning from executable queues and failed attempts", () => {
  expect(teamScope(card("draft"))).toBe("planned");
  expect(teamActivity(card("planned"))).toBe("Not started");
  expect(teamScope(card("queued"))).toBe("active");
  expect(teamScope(card("failed"))).toBe("active");
  expect(teamScope(card("completed"))).toBe("completed");
  expect(teamAttention(card("completed"))).toBe("—");
  expect(teamAttention(card("awaiting_review"))).toContain("Reviewer needed");
});
