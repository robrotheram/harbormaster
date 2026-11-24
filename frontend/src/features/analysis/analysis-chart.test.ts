import { expect, test } from "vitest";
import type { ChartSpec, DataPage } from "./api";
import { chartOptions } from "./analysis-chart";

const data = {
  columns: [
    { name: "x", type: "BIGINT" },
    { name: "y", type: "BIGINT" },
  ],
  rows: [
    [0, 12],
    [1000, 1],
  ],
  next_cursor: null,
  has_more: false,
  sampled: false,
  truncated: false,
} satisfies DataPage;

test.each(["line", "bar", "scatter", "histogram"] as const)(
  "renders %s options with numeric bin positions and no renderer options in the contract",
  (kind) => {
    const options = chartOptions(
      {
        chart_type: kind,
        encoding: {
          x: { field: "x", type: "quantitative" },
          y: { field: "y", type: "quantitative" },
        },
      },
      data,
    );
    expect(options).toMatchObject({
      xAxis: { type: "value" },
      tooltip: { renderMode: "richText" },
      series: [{ type: kind === "histogram" ? "bar" : kind, data: data.rows }],
    });
  },
);

test("normalizes a naive DuckDB timestamp to UTC and preserves null values", () => {
  const spec = {
    chart_type: "line",
    encoding: {
      x: { field: "x", type: "temporal" },
      y: { field: "y", type: "quantitative", label: "Reading count" },
    },
  } satisfies ChartSpec;
  const options = chartOptions(spec, {
    ...data,
    rows: [["2026-11-23T14:00:00", null]],
  });
  expect(options).toMatchObject({
    xAxis: { type: "time" },
    series: [
      { name: "Reading count", data: [[Date.UTC(2026, 10, 23, 14), null]] },
    ],
  });
});

test("colors nominal bars separately and applies chart grid settings", () => {
  const bars = {
    chart_type: "bar",
    encoding: {
      x: { field: "x", type: "nominal" },
      y: { field: "y", type: "quantitative" },
    },
  } satisfies ChartSpec;
  const page = {
    ...data,
    rows: [
      ["Black", 12],
      ["Cinnamon", 29],
    ],
  } satisfies DataPage;
  const defaults = chartOptions(bars, page, true);
  expect(defaults).toMatchObject({
    xAxis: { axisLabel: { color: "#b4becd" } },
    series: [
      {
        data: [
          { itemStyle: { color: "#7e9ef8" } },
          { itemStyle: { color: "#4cc5ad" } },
        ],
      },
    ],
  });
  const options = chartOptions(
    {
      ...bars,
      style: {
        category_colors: { Cinnamon: "#d98d59" },
        gridlines: { x: true, y: false },
      },
    },
    page,
    true,
  );
  expect(options).toMatchObject({
    xAxis: { splitLine: { show: true } },
    yAxis: { splitLine: { show: false } },
    series: [
      {
        data: [
          expect.anything(),
          { value: ["Cinnamon", 29], itemStyle: { color: "#d98d59" } },
        ],
      },
    ],
  });
  expect(chartOptions(bars, page, false)).toMatchObject({
    xAxis: { axisLabel: { color: "#4e5b6c" } },
  });
});

test("line series use the configured colors and show markers for short series", () => {
  const options = chartOptions(
    {
      chart_type: "line",
      encoding: {
        x: { field: "x", type: "quantitative" },
        y: { field: "y", type: "quantitative" },
        series: "group",
      },
      style: {
        series_colors: { "Above Ground": "#4cc5ad", "Ground Plane": "#e9ad64" },
      },
    },
    {
      ...data,
      columns: [...data.columns, { name: "group", type: "VARCHAR" }],
      rows: [
        [0, 3, "Above Ground"],
        [0, 9, "Ground Plane"],
        [1, 4, "Above Ground"],
      ],
    },
  );
  expect(options?.series).toMatchObject([
    {
      name: "Above Ground",
      lineStyle: { color: "#4cc5ad" },
      showSymbol: true,
      data: [
        [0, 3],
        [1, 4],
      ],
    },
    { name: "Ground Plane", lineStyle: { color: "#e9ad64" }, showSymbol: true },
  ]);
});

test("pie charts combine matching categories and require complete, nonnegative totals", () => {
  const pie = {
    chart_type: "pie",
    encoding: { category: "x", value: "y" },
    style: { category_colors: { Black: "#768498", Gray: "#aebdcc" } },
  } satisfies ChartSpec;
  const page = {
    ...data,
    rows: [
      ["Black", 12],
      ["Gray", 29],
      ["Black", 3],
      ["Cinnamon", null],
      ["Not recorded", 0],
    ],
  } satisfies DataPage;
  expect(chartOptions(pie, page)).toMatchObject({
    legend: { type: "scroll" },
    tooltip: { trigger: "item", formatter: "{b}\n{c} ({d}%)" },
    series: [
      {
        type: "pie",
        data: [
          { name: "Black", value: 15, itemStyle: { color: "#768498" } },
          { name: "Gray", value: 29, itemStyle: { color: "#aebdcc" } },
        ],
      },
    ],
  });
  expect(chartOptions(pie, { ...page, truncated: true })).toBeUndefined();
  expect(chartOptions(pie, { ...page, rows: [["Black", -1]] })).toBeUndefined();
  expect(chartOptions(pie, { ...page, rows: [["Black", 0]] })).toBeUndefined();
});
