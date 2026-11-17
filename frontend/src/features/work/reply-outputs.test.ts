import { expect, it } from "vitest";
import { replyOutputs } from "./reply-outputs";
it("finds explicit data and image outputs without treating code or external URLs as files", () => {
  expect(
    replyOutputs(
      "[data](out/data.csv) [again](out/data.csv) ![image](out/plot.svg) [external](https://example.com/data.csv) [secret](../private.json) `plain.json`\n```text\n[fake](fake.csv)\n```",
    ),
  ).toEqual([
    { name: "out/data.csv", path: "out/data.csv" },
    { name: "out/plot.svg", path: "out/plot.svg" },
  ]);
});
it("keeps inline chart specifications available to the workspace data action", () => {
  expect(replyOutputs('```workbench-chart\n{"version":1}\n```')).toEqual([
    { name: "Chart 1", source: '{"version":1}' },
  ]);
});
