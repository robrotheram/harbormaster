import { useEffect, useMemo, useRef, useState } from "react";
import type { EChartsCoreOption } from "echarts/core";
import { createChart } from "@/features/work/chart-engine";
import {
  finite,
  type Axis,
  type Cell,
  type ChartSpec,
  type DataPage,
} from "./api";

const palettes = {
  dark: ["#7e9ef8", "#4cc5ad", "#e9ad64", "#baa0ee", "#ed8da3", "#64bfd3"],
  light: ["#476bd2", "#168975", "#a55b18", "#7e5bbd", "#b74e72", "#217d96"],
};

function chartTheme(dark: boolean) {
  return {
    palette: dark ? palettes.dark : palettes.light,
    text: dark ? "#e2e6ed" : "#293240",
    label: dark ? "#b4becd" : "#4e5b6c",
    grid: dark ? "#343d49" : "#dbe2eb",
    border: dark ? "#576475" : "#a0adbd",
    surface: dark ? "#242a33" : "#ffffff",
  };
}

function colorFor(
  colors: Record<string, string> | undefined,
  name: string,
  index: number,
  palette: string[],
) {
  return colors && Object.hasOwn(colors, name)
    ? colors[name]
    : palette[index % palette.length];
}

function axisValue(value: Cell, axis: Axis) {
  if (axis.type === "quantitative") return finite(value);
  if (axis.type === "temporal") {
    if (typeof value !== "string") return null;
    const date = /^\d{4}-\d\d-\d\dT[\d:.]+$/.test(value) ? value + "Z" : value;
    const timestamp = Date.parse(date);
    return Number.isFinite(timestamp) ? timestamp : null;
  }
  return value === null ? null : String(value);
}

function pieOptions(
  spec: Extract<ChartSpec, { chart_type: "pie" }>,
  page: DataPage,
  theme: ReturnType<typeof chartTheme>,
) {
  if (page.sampled || page.truncated || page.has_more) return;
  const category = page.columns.findIndex(
    ({ name }) => name === spec.encoding.category,
  );
  const value = page.columns.findIndex(
    ({ name }) => name === spec.encoding.value,
  );
  const totals = new Map<string, number>();
  for (const row of page.rows) {
    const amount = finite(row[value]);
    if (amount === null || amount === 0) continue;
    if (amount < 0) return;
    const name = String(row[category] ?? "Not recorded");
    totals.set(name, (totals.get(name) ?? 0) + amount);
  }
  if (
    !totals.size ||
    !Number.isFinite(
      [...totals.values()].reduce((sum, value) => sum + value, 0),
    )
  )
    return;
  return {
    tooltip: { trigger: "item", formatter: "{b}\n{c} ({d}%)" },
    legend: {
      type: "scroll",
      bottom: 0,
      left: "center",
      textStyle: { color: theme.label, width: 120, overflow: "truncate" },
    },
    series: [
      {
        type: "pie",
        radius: "67%",
        center: ["50%", "44%"],
        itemStyle: { borderColor: theme.surface, borderWidth: 1.5 },
        label: {
          color: theme.text,
          formatter: "{d}%",
        },
        labelLine: {
          lineStyle: { color: theme.border },
          length: 8,
          length2: 6,
        },
        minShowLabelAngle: 18,
        data: [...totals].map(([name, value], index) => ({
          name,
          value,
          itemStyle: {
            color: colorFor(
              spec.style?.category_colors,
              name,
              index,
              theme.palette,
            ),
          },
        })),
      },
    ],
  };
}

export function chartOptions(
  spec: ChartSpec,
  page: DataPage,
  dark = false,
): EChartsCoreOption | undefined {
  const theme = chartTheme(dark);
  const base = {
    animation: false,
    aria: { enabled: true },
    textStyle: { fontFamily: "Geist Variable, sans-serif", color: theme.text },
  };
  const tooltip = {
    renderMode: "richText",
    confine: true,
    backgroundColor: theme.surface,
    borderColor: theme.border,
    textStyle: { color: theme.text },
  };
  if (spec.chart_type === "pie") {
    const options = pieOptions(spec, page, theme);
    return (
      options && {
        ...base,
        ...options,
        tooltip: { ...tooltip, ...options.tooltip },
      }
    );
  }

  const { x, y, series } = spec.encoding;
  const indexOf = (field: string) =>
    page.columns.findIndex(({ name }) => field === name);
  const xIndex = indexOf(x.field);
  const yIndex = indexOf(y.field);
  const seriesIndex = series ? indexOf(series) : -1;
  const histogram = spec.chart_type === "histogram";
  const category = x.type === "nominal";
  const categoryBars =
    spec.chart_type === "bar" &&
    category &&
    !series &&
    !spec.style?.series_colors;
  const groups = new Map<string, [string | number, number | null][]>();
  const labels = new Set<string>();
  for (const row of page.rows) {
    const key =
      seriesIndex < 0 ? y.label || y.field : String(row[seriesIndex] ?? "null");
    const value = axisValue(row[xIndex], x);
    if (value === null) continue;
    const seriesRows = groups.get(key) || [];
    if (category) labels.add(String(value));
    seriesRows.push([category ? String(value) : value, finite(row[yIndex])]);
    groups.set(key, seriesRows);
  }
  const axisLabel = { color: theme.label, hideOverlap: true, fontSize: 12 };
  const axisLine = { lineStyle: { color: theme.border } };
  const axisName = { color: theme.text, fontSize: 12 };
  const splitLine = (show: boolean) => ({
    show,
    lineStyle: { color: theme.grid },
  });
  const categoryOrder = new Map(
    [...labels].map((label, index) => [label, index]),
  );
  return {
    ...base,
    grid: {
      left: 12,
      right: 24,
      top: series ? 57 : 30,
      bottom: 42,
      containLabel: true,
    },
    xAxis: {
      type: category ? "category" : x.type === "temporal" ? "time" : "value",
      data: category ? [...labels] : undefined,
      name: x.label || x.field.replaceAll("_", " "),
      scale: x.type === "quantitative",
      boundaryGap: histogram ? ["5%", "5%"] : undefined,
      nameLocation: "middle",
      nameGap: 32,
      nameTextStyle: axisName,
      axisLabel: { ...axisLabel, width: 105, overflow: "truncate" },
      axisLine,
      axisTick: { show: false },
      splitLine: splitLine(spec.style?.gridlines?.x ?? false),
    },
    yAxis: {
      type: "value",
      name: y.label || y.field.replaceAll("_", " "),
      nameLocation: "end",
      nameTextStyle: { ...axisName, align: "left" },
      axisLabel,
      axisTick: { show: false },
      splitLine: splitLine(spec.style?.gridlines?.y ?? true),
    },
    tooltip: {
      ...tooltip,
      trigger: spec.chart_type === "scatter" ? "item" : "axis",
      axisPointer: {
        type: spec.chart_type === "bar" && category ? "shadow" : "line",
      },
    },
    legend: series
      ? { type: "scroll", top: 0, textStyle: { color: theme.label } }
      : undefined,
    series: [...groups].map(([name, data], index) => {
      const color = colorFor(
        spec.style?.series_colors,
        name,
        index,
        theme.palette,
      );
      return {
        type: histogram ? "bar" : spec.chart_type,
        name,
        data: categoryBars
          ? data.map(([label, value]) => ({
              value: [label, value],
              itemStyle: {
                color: colorFor(
                  spec.style?.category_colors,
                  String(label),
                  categoryOrder.get(String(label)) ?? 0,
                  theme.palette,
                ),
              },
            }))
          : data,
        itemStyle: {
          color,
          borderRadius: spec.chart_type === "bar" ? 3 : undefined,
        },
        barCategoryGap: histogram ? 0 : undefined,
        barMaxWidth: spec.chart_type === "bar" ? 60 : undefined,
        lineStyle: { color, width: 2.75 },
        symbol: "circle",
        symbolSize: 6,
        showSymbol: spec.chart_type === "line" && data.length <= 14,
        emphasis: { focus: series ? "series" : "self" },
      };
    }),
  };
}

export function AnalysisChart({
  spec,
  page,
  title,
}: {
  spec: ChartSpec;
  page: DataPage;
  title: string;
}) {
  const host = useRef<HTMLDivElement>(null);
  const [error, setError] = useState(false);
  const [dark, setDark] = useState(() =>
    document.documentElement.classList.contains("dark"),
  );
  const options = useMemo(
    () => chartOptions(spec, page, dark),
    [spec, page, dark],
  );
  useEffect(() => {
    const root = document.documentElement;
    const update = () => setDark(root.classList.contains("dark"));
    const observer = new MutationObserver(update);
    observer.observe(root, { attributes: true, attributeFilter: ["class"] });
    update();
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (!host.current || !options) return;
    const node = host.current;
    let chart: ReturnType<typeof createChart> | undefined;
    let resize: ResizeObserver | undefined;
    try {
      chart = createChart(node);
      chart.setOption(options);
      resize = new ResizeObserver(() => chart?.resize());
      resize.observe(node);
    } catch {
      setError(true);
    }
    return () => {
      resize?.disconnect();
      chart?.dispose();
    };
  }, [options]);
  if (!page.rows.length)
    return (
      <p className="text-sm text-muted-foreground">
        No chart rows are available.
      </p>
    );
  if (!options)
    return (
      <p role="status" className="text-sm text-muted-foreground">
        {page.sampled || page.truncated || page.has_more
          ? "Pie charts need complete totals. Aggregate the data by category."
          : "Pie charts need nonnegative values and a positive total."}
      </p>
    );
  if (error)
    return <p role="status">The chart renderer could not draw this data.</p>;
  return (
    <div ref={host} role="img" aria-label={title} className="h-80 min-w-0" />
  );
}
