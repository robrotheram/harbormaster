import { describe, it, expect } from "vitest";
import { stages, workStage, workStatus, workFinished } from "./stages";
import { workActions } from "./api";
import { stateTone } from "@/shared/format";

describe("work delivery presentation", () => {
  it("distinguishes awaiting review from the backend's completed acceptance state", () => {
    for (const state of ["awaiting_review", "review_required"]) {
      expect(workStage(state)).toBe("review");
      expect(workActions(state).review).toBe(true);
    }
    // ReviewWork stores completed only after the human accepts the result.
    for (const state of ["completed", "accepted"]) {
      expect(workStage(state)).toBe("done");
      expect(workActions(state)).toEqual({
        run: false,
        cancel: false,
        review: false,
      });
      expect(stateTone(state)).toBe("good");
    }
  });
  it("surfaces failed or unknown work as needing attention, never delivered", () => {
    for (const state of [
      "failed",
      "blocked",
      "rejected",
      "cancelled",
      "future_state",
    ]) {
      expect(workStage(state)).toBe("blocked");
    }
  });
});

it("groups closed and accepted work under Done without confusing their outcomes", () => {
  expect(stages.find((s) => s.id === "done")?.label).toBe("Done");
  expect(workStatus("closed")).toBe("closed");
  expect(workStatus("completed")).toBe("accepted");
  for (const state of ["closed", "completed", "accepted"]) {
    expect(workFinished(state)).toBe(true);
    expect(workStage(state)).toBe("done");
  }
  expect(workFinished("running")).toBe(false);
  expect(workActions("closed").review).toBe(false);
});

it("reopened planning states are runnable without claiming an agent is running", () => {
  expect(workStage("planned")).toBe("queued");
  expect(workStage("in_progress")).toBe("running");
  for (const state of ["planned", "in_progress"]) {
    expect(workActions(state)).toEqual({
      run: true,
      cancel: false,
      review: false,
    });
    expect(workFinished(state)).toBe(false);
  }
});
