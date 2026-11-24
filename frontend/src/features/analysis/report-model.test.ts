import { expect, test } from "vitest";
import {
  reportMarkdown,
  reportReferences,
  reportSnippet,
} from "./report-model";

test("authored reports resolve only real visualization blocks", () => {
  const markdown = [
    "````md",
    "```harbormaster-visualization",
    "example",
    "```",
    "````",
    "```harbormaster-visualization",
    "chart_live",
    "```",
    "~~~harbormaster-visualization",
    "ws_other/geo_live",
    "~~~",
    "```harbormaster-visualization",
    "../secret",
    "```",
  ].join("\n");
  expect(reportReferences(markdown, "ws_one")).toEqual([
    { worker_id: "ws_one", artifact_id: "chart_live" },
    { worker_id: "ws_other", artifact_id: "geo_live" },
  ]);
});

test("repeated copied snippets preserve placement and resolve the same result once", () => {
  const table = { worker_id: "ws_one", artifact_id: "table_one" };
  const snippet = reportSnippet(table);
  const markdown = reportMarkdown(
    "Report",
    `${snippet}\n\nCompare with the first table.\n\n${snippet}`,
  );
  expect(markdown.split(snippet)).toHaveLength(3);
  expect(reportReferences(markdown, "another_worker")).toEqual([table]);
});
