import { describe, expect, it } from "vitest";
import { numericColumns, parseVisualData } from "./visual-data";
describe("visual data", () => {
  it("reads quoted CSV cells, escaped quotes, multiline labels and negative values", () => {
    const rows = parseVisualData(
      'label,value\n"one, two",-2\n"a ""quote""\non two lines",3.5',
    );
    expect(rows).toEqual([
      { label: "one, two", value: "-2" },
      { label: 'a "quote"\non two lines', value: "3.5" },
    ]);
    expect(numericColumns(rows)).toEqual(["value"]);
  });
  it("rejects incomplete CSV and distinguishes missing values from zero", () => {
    expect(() => parseVisualData('label,value\n"unfinished,2')).toThrow();
    expect(numericColumns(parseVisualData("label,value\na,0\nb,"))).toEqual([]);
  });
  it("ignores nested JSON and bounds source size and preserves rows", () => {
    expect(
      parseVisualData('[{"name":"trial","score":2,"nested":{"x":1}}]'),
    ).toEqual([{ name: "trial", score: 2 }]);
    expect(
      parseVisualData(
        JSON.stringify(Array.from({ length: 600 }, () => ({ v: 1 }))),
      ),
    ).toHaveLength(600);
    expect(() => parseVisualData(" ".repeat(262145))).toThrow();
  });
});
