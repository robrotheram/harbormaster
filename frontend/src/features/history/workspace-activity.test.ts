import { describe, expect, it } from "vitest";
import { backgroundJobDetails } from "@/features/history/workspace-activity";
describe("workspace background jobs", () => {
  it("names the confirmed Matrix listener and leaves additional jobs visible", () => {
    expect(
      backgroundJobDetails(1, [
        { name: "Matrix listener", state: "connected" },
      ]),
    ).toEqual({ services: ["Matrix listener: Connected"], remaining: 0 });
    expect(
      backgroundJobDetails(2, [{ name: "Matrix listener", state: "connected" }])
        .remaining,
    ).toBe(1);
  });
  it("does not attribute an unknown process to a disconnected or failed listener", () => {
    for (const state of ["failed", "disconnected"]) {
      expect(
        backgroundJobDetails(1, [{ name: "Matrix listener", state }]).remaining,
      ).toBe(1);
    }
    expect(backgroundJobDetails(1, [])).toEqual({ services: [], remaining: 1 });
  });
  it("only uses recognised service names and states", () => {
    expect(
      backgroundJobDetails(1, [
        { name: "unexpected process command", state: "connected" },
        { name: "Matrix listener", state: "unknown" },
      ]),
    ).toEqual({ services: [], remaining: 1 });
  });
});
