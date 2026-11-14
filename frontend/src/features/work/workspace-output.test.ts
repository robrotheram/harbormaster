import { describe, expect, it } from "vitest";
import { chartPath } from "./workspace-output";

describe("message chart links", () => {
  it("only embeds explicitly named chart files", () => {
    expect(chartPath("outputs/results.chart.json")).toBe(
      "outputs/results.chart.json",
    );
    expect(chartPath("./outputs/results.chart.json")).toBe(
      "outputs/results.chart.json",
    );
    for (const path of [
      "outputs/result.json",
      "outputs/results.csv",
      "package.json",
      "http://1.1.1.1:80",
      "Try http://1.1.1.1:80",
    ]) {
      expect(chartPath(path)).toBeUndefined();
    }
  });
  it("keeps remote and unsafe chart paths out of embeds", () => {
    for (const path of [
      "https://example.com/results.chart.json",
      "../results.chart.json",
      "/results.chart.json",
      "%2e%2e/results.chart.json",
    ]) {
      expect(chartPath(path)).toBeUndefined();
    }
  });
});
