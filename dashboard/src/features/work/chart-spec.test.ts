import { describe, it, expect } from "vitest";
import {
  parseChartSpec,
  numberValue,
  metricColumns,
  formatValue,
  csvData,
} from "./chart-spec";
const sample = {
  version: 1,
  title: "Storage",
  type: "donut",
  x: "name",
  y: ["bytes"],
  unit: "bytes",
  data: [
    { name: "Used", bytes: 1024 },
    { name: "Available", bytes: 8192 },
  ],
};
describe("browser chart contract", () => {
  it("validates declarations and infers CSV/JSON datasets without treating missing data as zero", () => {
    expect(parseChartSpec(JSON.stringify(sample))).toEqual(sample);
    const spec = parseChartSpec("day,value\nMonday,0\nTuesday,\nWednesday,4");
    expect(spec.y).toEqual(["value"]);
    expect(numberValue(spec.data[1].value)).toBeNull();
    expect(
      metricColumns([
        { x: "a", score: null },
        { x: "b", score: 3 },
      ]),
    ).toEqual(["score"]);
    expect(
      parseChartSpec('[{"step":1,"score":2},{"step":2,"score":3}]').x,
    ).toBe("step");
  });
  it("rejects executable/library settings, bad encodings and misleading donut values", () => {
    for (const patch of [
      { version: 2 },
      { url: "https://outside.test/data" },
      { tooltip: { formatter: "code" } },
      { y: ["unknown"] },
      { type: "scatter" },
      { data: [{ name: "Used", bytes: -1 }] },
      { data: [{ name: "Used", bytes: { nested: 1 } }] },
    ])
      expect(() =>
        parseChartSpec(JSON.stringify({ ...sample, ...patch })),
      ).toThrow();
    expect(() =>
      parseChartSpec(
        '{"version":1,"title":"x","type":"bar","x":"name","y":["value"],"data":[{"name":"a","value":1e999}]}',
      ),
    ).toThrow();
  });
  it("bounds datasets and preserves original rows", () => {
    expect(() =>
      parseChartSpec(
        JSON.stringify(Array.from({ length: 2001 }, () => ({ x: 1, y: 2 }))),
      ),
    ).toThrow(/2,000/);
    expect(() => parseChartSpec(" ".repeat(262145))).toThrow(/256/);
    const spec = parseChartSpec(
      JSON.stringify(
        Array.from({ length: 600 }, (_, i) => ({ x: i, y: i + 1 })),
      ),
    );
    expect(spec.data).toHaveLength(600);
  });
  it("formats units and exports quoted spreadsheet-safe cells", () => {
    expect(formatValue(1024, "bytes")).toBe("1 KiB");
    expect(numberValue(" ")).toBeNull();
    expect(
      csvData([{ name: '=HYPERLINK("url")', value: -2 }], ["name", "value"]),
    ).toContain('"\'=HYPERLINK(""url"")","-2"');
  });
});
