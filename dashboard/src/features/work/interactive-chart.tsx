import { useEffect, useMemo, useRef, useState } from "react";
import type { EChartsType, EChartsCoreOption } from "echarts/core";
import {
  Download,
  SlidersHorizontal,
  Table2,
  ChartNoAxesCombined,
  MessageSquare,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  chartTypes,
  csvData,
  formatValue,
  metricColumns,
  numberValue,
  type ChartRow,
  type ChartSpec,
  type ChartType,
} from "./chart-spec";

function download(body: string, type: string, filename: string) {
  const url = URL.createObjectURL(new Blob([body], { type }));
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function InteractiveChart({
  spec,
  onAsk,
}: {
  spec: ChartSpec;
  onAsk?: (message: string) => void;
}) {
  const [kind, setKind] = useState<ChartType>(spec.type);
  const [x, setX] = useState(spec.x),
    [ys, setYs] = useState(spec.y);
  const [explore, setExplore] = useState(false),
    [table, setTable] = useState(false);
  const [filter, setFilter] = useState(""),
    [sort, setSort] = useState("source");
  const [selected, setSelected] = useState<{
    row: ChartRow;
    metric: string;
  } | null>(null);
  const [ready, setReady] = useState(false),
    [error, setError] = useState("");
  const [theme, setTheme] = useState(
    document.documentElement.classList.contains("dark"),
  );
  const host = useRef<HTMLDivElement>(null),
    chart = useRef<EChartsType | null>(null);
  const columns = useMemo(
    () => [...new Set(spec.data.flatMap(Object.keys))],
    [spec.data],
  );
  const metrics = useMemo(() => metricColumns(spec.data), [spec.data]);
  const rows = useMemo(() => {
    const result = spec.data.filter((r) =>
      Object.values(r).some((v) =>
        String(v ?? "")
          .toLowerCase()
          .includes(filter.toLowerCase()),
      ),
    );
    if (sort !== "source")
      result.sort(
        (a, b) =>
          ((numberValue(a[ys[0]]) ?? -Infinity) -
            (numberValue(b[ys[0]]) ?? -Infinity)) *
          (sort === "desc" ? -1 : 1),
      );
    return result;
  }, [spec.data, filter, sort, ys]);
  const unavailable =
    kind === "scatter" && !metrics.includes(x)
      ? "Choose a numeric horizontal axis for a scatter plot."
      : kind === "donut" &&
          (ys.length !== 1 ||
            rows.some((r) => (numberValue(r[ys[0]]) ?? 0) < 0))
        ? "Choose one nonnegative metric for a donut chart."
        : "";
  useEffect(() => {
    const observer = new MutationObserver(() =>
      setTheme(document.documentElement.classList.contains("dark")),
    );
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    let cancelled = false;
    let observer: ResizeObserver | undefined;
    import("./chart-engine")
      .then(({ createChart }) => {
        if (cancelled || !host.current) return;
        chart.current = createChart(host.current);
        setReady(true);
        observer = new ResizeObserver(() => chart.current?.resize());
        observer.observe(host.current);
      })
      .catch(() => {
        if (!cancelled)
          setError(
            "The chart renderer could not load. Reload this page to try again.",
          );
      });
    return () => {
      cancelled = true;
      observer?.disconnect();
      chart.current?.dispose();
      chart.current = null;
    };
  }, []);
  useEffect(() => {
    setSelected(null);
  }, [filter, sort, x, ys, kind]);
  useEffect(() => {
    const view = chart.current;
    if (!view || !ready || unavailable) return;
    const text = theme ? "#d4d4d8" : "#303038",
      grid = theme ? "#303038" : "#e4e4e7";
    const colors = [
      "#5e82f7",
      "#34b99b",
      "#c18d50",
      "#a389d4",
      "#d87586",
      "#56a8b8",
    ];
    const options: EChartsCoreOption = {
      animation: false,
      color: colors,
      textStyle: { fontFamily: "Geist Variable, sans-serif", color: text },
      aria: {
        enabled: true,
        label: { description: `${ys.join(", ")} by ${x}` },
      },
      tooltip: {
        trigger: kind === "donut" || kind === "scatter" ? "item" : "axis",
        renderMode: "richText",
        confine: true,
        valueFormatter: (v: unknown) =>
          typeof v === "number"
            ? formatValue(v, spec.unit)
            : String(v ?? "Missing"),
      },
      legend: {
        show: ys.length > 1,
        type: "scroll",
        top: 0,
        textStyle: { color: text },
      },
      ...(kind === "donut"
        ? {}
        : {
            grid: {
              left: 16,
              right: 24,
              top: ys.length > 1 ? 48 : 20,
              bottom: rows.length > 12 ? 65 : 35,
              containLabel: true,
            },
            xAxis: {
              type: kind === "scatter" ? "value" : "category",
              ...(kind === "scatter"
                ? {}
                : { data: rows.map((r) => String(r[x] ?? "Missing")) }),
              axisLabel: {
                color: text,
                hideOverlap: true,
                width: 110,
                overflow: "truncate",
              },
              axisLine: { lineStyle: { color: grid } },
            },
            yAxis: {
              type: "value",
              axisLabel: {
                color: text,
                formatter: (n: number) => formatValue(n, spec.unit),
              },
              splitLine: { lineStyle: { color: grid } },
            },
            dataZoom:
              rows.length > 12
                ? [
                    { type: "inside", xAxisIndex: 0 },
                    {
                      type: "slider",
                      xAxisIndex: 0,
                      height: 20,
                      bottom: 4,
                      textStyle: { color: text },
                    },
                  ]
                : [],
          }),
      series:
        kind === "donut"
          ? [
              {
                name: ys[0],
                type: "pie",
                stillShowZeroSum: false,
                radius: ["48%", "72%"],
                label: {
                  color: text,
                  overflow: "truncate",
                  width: 120,
                  formatter: "{b}",
                },
                data: rows.flatMap((r, i) =>
                  numberValue(r[ys[0]]) === null
                    ? []
                    : [
                        {
                          name: String(r[x] ?? "Missing"),
                          value: numberValue(r[ys[0]]),
                          rowIndex: i,
                        },
                      ],
                ),
              },
            ]
          : ys.map((metric) => ({
              name: metric,
              type: kind === "area" ? "line" : kind,
              connectNulls: false,
              showSymbol: rows.length <= 80,
              symbolSize: kind === "scatter" ? 9 : 6,
              ...(kind === "area" ? { areaStyle: { opacity: 0.14 } } : {}),
              data: rows.map((r, i) => ({
                name: String(r[x] ?? "Missing"),
                value:
                  kind === "scatter"
                    ? [numberValue(r[x]), numberValue(r[metric])]
                    : numberValue(r[metric]),
                rowIndex: i,
              })),
            })),
    };
    view.setOption(options, true);
    view.off("click");
    view.on("click", (event) => {
      const detail = event as {
        data?: { rowIndex?: number };
        seriesName?: string;
      };
      const index = detail.data?.rowIndex;
      if (typeof index === "number" && rows[index])
        setSelected({ row: rows[index], metric: detail.seriesName || ys[0] });
    });
    view.resize();
  }, [ready, rows, x, ys, kind, spec.unit, spec.title, theme, unavailable]);
  useEffect(() => {
    if (!table) chart.current?.resize();
  }, [table]);
  return (
    <section className="interactive-chart" aria-label={spec.title}>
      <header className="chart-heading">
        <div>
          <h3>{spec.title}</h3>
          {spec.description && <p>{spec.description}</p>}
        </div>
        <div className="chart-actions">
          <Button
            variant="ghost"
            size="sm"
            aria-expanded={explore}
            onClick={() => setExplore(!explore)}
          >
            <SlidersHorizontal size={14} />
            Explore
          </Button>
          <Button
            variant="ghost"
            size="sm"
            aria-pressed={table}
            onClick={() => setTable(!table)}
          >
            {table ? <ChartNoAxesCombined size={14} /> : <Table2 size={14} />}{" "}
            {table ? "Chart" : "Data"}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label="Export chart">
                <Download size={14} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                onSelect={() =>
                  download(
                    csvData(rows, columns),
                    "text/csv;charset=utf-8",
                    "chart-data.csv",
                  )
                }
              >
                Filtered rows (CSV)
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={() =>
                  download(
                    JSON.stringify(spec, null, 2),
                    "application/json",
                    "chart-data.json",
                  )
                }
              >
                Original chart and data (JSON)
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={!ready || !!unavailable || !rows.length}
                onSelect={() => {
                  if (chart.current)
                    download(
                      chart.current.renderToSVGString(),
                      "image/svg+xml",
                      "chart.svg",
                    );
                }}
              >
                Current chart (SVG)
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>
      {explore && (
        <div className="chart-explorer">
          <label>
            Chart
            <select
              aria-label="Chart type"
              value={kind}
              onChange={(e) => {
                setKind(e.target.value as ChartType);
                if (e.target.value === "donut") setYs(ys.slice(0, 1));
              }}
            >
              {chartTypes.map((t) => (
                <option key={t} value={t}>
                  {
                    {
                      bar: "Bars",
                      line: "Line",
                      area: "Area",
                      scatter: "Scatter",
                      donut: "Donut",
                    }[t]
                  }
                </option>
              ))}
            </select>
          </label>
          <label>
            Horizontal axis
            <select
              aria-label="Chart horizontal axis"
              value={x}
              onChange={(e) => setX(e.target.value)}
            >
              {columns.map((c) => (
                <option key={c}>{c}</option>
              ))}
            </select>
          </label>
          <label>
            Value
            <select
              aria-label="Chart value"
              value={ys[0]}
              onChange={(e) => setYs([e.target.value])}
            >
              {metrics.map((c) => (
                <option key={c}>{c}</option>
              ))}
            </select>
          </label>
          <label>
            Order
            <select
              aria-label="Chart order"
              value={sort}
              onChange={(e) => setSort(e.target.value)}
            >
              <option value="source">Source order</option>
              <option value="desc">Highest first</option>
              <option value="asc">Lowest first</option>
            </select>
          </label>
          <label className="chart-filter">
            Filter rows
            <Input
              aria-label="Filter chart rows"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Search the data…"
            />
          </label>
          {metrics.length > 1 && kind !== "donut" && (
            <fieldset>
              <legend>Compare metrics</legend>
              {metrics.map((m) => (
                <label key={m}>
                  <input
                    type="checkbox"
                    checked={ys.includes(m)}
                    disabled={ys.includes(m) ? ys.length === 1 : ys.length >= 6}
                    onChange={() =>
                      setYs(
                        ys.includes(m) ? ys.filter((y) => y !== m) : [...ys, m],
                      )
                    }
                  />
                  {m}
                </label>
              ))}
            </fieldset>
          )}
        </div>
      )}
      {error && <p role="alert">{error}</p>}
      {!table && unavailable && (
        <p role="status" className="chart-hint">
          {unavailable}
        </p>
      )}
      {!rows.length && (
        <p role="status" className="chart-hint">
          No rows match this filter.
        </p>
      )}
      <div
        className="browser-chart"
        ref={host}
        role="img"
        aria-label={`${ys.join(", ")} by ${x}`}
        hidden={table || !!unavailable || !rows.length}
      />
      {!ready && !error && !table && (
        <p role="status">Loading interactive chart…</p>
      )}
      {table && !!rows.length && (
        <div className="chart-table">
          <table>
            <thead>
              <tr>
                <th>
                  <span className="sr-only">Inspect</span>
                </th>
                {columns.map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i} aria-selected={selected?.row === r}>
                  <td>
                    <button
                      aria-label={`Inspect row ${i + 1}`}
                      onClick={() => setSelected({ row: r, metric: ys[0] })}
                    >
                      {i + 1}
                    </button>
                  </td>
                  {columns.map((c) => (
                    <td key={c}>
                      {r[c] === null || r[c] === undefined ? "—" : String(r[c])}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {selected && (
        <div className="chart-selection">
          <p>
            <strong>{String(selected.row[x] ?? "Selected row")}</strong> ·{" "}
            {selected.metric}:{" "}
            {numberValue(selected.row[selected.metric]) === null
              ? "Missing"
              : formatValue(
                  numberValue(selected.row[selected.metric])!,
                  spec.unit,
                )}
          </p>
          {onAsk && (
            <Button
              size="sm"
              variant="outline"
              onClick={() =>
                onAsk(
                  `Investigate this point in “${spec.title}” (${selected.metric}${spec.unit ? `, ${spec.unit}` : ""}): ${JSON.stringify(selected.row).slice(0, 6000)}. Explain what it means and any useful next checks.\n`,
                )
              }
            >
              <MessageSquare size={14} />
              Ask agent about this
            </Button>
          )}
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label="Clear chart selection"
            onClick={() => setSelected(null)}
          >
            <X size={14} />
          </Button>
        </div>
      )}
      <footer className="chart-footer">
        <span>
          {rows.length} of {spec.data.length} rows
        </span>
        <span>
          Hover for values · Select a point to investigate
          {rows.length > 12 ? " · Drag to zoom" : ""}
        </span>
      </footer>
    </section>
  );
}
