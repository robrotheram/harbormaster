import { parseVisualData } from "./visual-data";
export type ChartType = "bar" | "line" | "area" | "scatter" | "donut";
export type ChartRow = Record<string, string | number | null>;
export type ChartSpec = {
  version: 1;
  title: string;
  description?: string;
  type: ChartType;
  x: string;
  y: string[];
  unit?: string;
  data: ChartRow[];
};
export const chartTypes: ChartType[] = [
  "bar",
  "line",
  "area",
  "scatter",
  "donut",
];
export function numberValue(value: unknown): number | null {
  if (value === null || value === undefined || String(value).trim() === "")
    return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}
export function metricColumns(rows: ChartRow[]) {
  const keys = [...new Set(rows.flatMap(Object.keys))];
  return keys.filter(
    (k) =>
      rows.some((r) => numberValue(r[k]) !== null) &&
      rows.every(
        (r) =>
          r[k] == null ||
          String(r[k]).trim() === "" ||
          numberValue(r[k]) !== null,
      ),
  );
}
function validateRows(value: unknown): ChartRow[] {
  if (!Array.isArray(value) || !value.length || value.length > 2000)
    throw new Error("Use between 1 and 2,000 rows.");
  const keys = new Set<string>();
  for (const row of value) {
    if (!row || typeof row !== "object" || Array.isArray(row))
      throw new Error("Each data row must be an object.");
    for (const [k, v] of Object.entries(row)) {
      keys.add(k);
      if (
        !k ||
        k.length > 100 ||
        ["__proto__", "constructor", "prototype"].includes(k)
      )
        throw new Error("Invalid column name.");
      if (!(
        v === null ||
        (typeof v === "string" && v.length <= 2000) ||
        (typeof v === "number" && Number.isFinite(v))
      ))
        throw new Error("Cells must be finite numbers, text, or null.");
    }
  }
  if (keys.size < 2 || keys.size > 32)
    throw new Error("Use between 2 and 32 columns.");
  return value as ChartRow[];
}
export function parseChartSpec(
  source: string,
  title = "Explore data",
): ChartSpec {
  if (new TextEncoder().encode(source).length > 262144)
    throw new Error("Use data smaller than 256 KiB.");
  const text = source.trim();
  const parsed: unknown = /^[\[{]/.test(text)
    ? JSON.parse(text)
    : parseVisualData(text);
  if (Array.isArray(parsed)) {
    const data = validateRows(parsed),
      metrics = metricColumns(data);
    if (!metrics.length)
      throw new Error("Include at least one numeric column.");
    const x =
      Object.keys(data[0]).find((k) => !metrics.includes(k)) ||
      Object.keys(data[0])[0];
    return {
      version: 1,
      title,
      type: "bar",
      x,
      y: [metrics.find((k) => k !== x) || metrics[0]],
      data,
    };
  }
  if (!parsed || typeof parsed !== "object")
    throw new Error("Use a chart specification, JSON rows, or CSV.");
  const spec = parsed as Record<string, unknown>;
  if (
    Object.keys(spec).some(
      (k) =>
        ![
          "version",
          "title",
          "description",
          "type",
          "x",
          "y",
          "unit",
          "data",
        ].includes(k),
    )
  )
    throw new Error(
      "Unsupported chart setting. Use the Workbench chart format.",
    );
  if (spec.version !== 1 || !chartTypes.includes(spec.type as ChartType))
    throw new Error("Use chart version 1 and a supported chart type.");
  if (
    typeof spec.title !== "string" ||
    !spec.title.trim() ||
    spec.title.length > 200
  )
    throw new Error("Give the chart a short title.");
  if (
    spec.description !== undefined &&
    (typeof spec.description !== "string" || spec.description.length > 1000)
  )
    throw new Error("Use a description of up to 1,000 characters.");
  if (
    spec.unit !== undefined &&
    (typeof spec.unit !== "string" || spec.unit.length > 24)
  )
    throw new Error("Use a short unit label.");
  const data = validateRows(spec.data),
    columns = new Set(data.flatMap(Object.keys)),
    metrics = metricColumns(data);
  if (typeof spec.x !== "string" || !columns.has(spec.x))
    throw new Error("The horizontal axis must name a data column.");
  if (
    !Array.isArray(spec.y) ||
    !spec.y.length ||
    spec.y.length > 6 ||
    new Set(spec.y).size !== spec.y.length ||
    !spec.y.every((y) => typeof y === "string" && metrics.includes(y))
  )
    throw new Error("Choose 1–6 numeric columns for y.");
  if (spec.type === "scatter" && !metrics.includes(spec.x))
    throw new Error("Scatter plots need a numeric horizontal axis.");
  const y = spec.y as string[];
  if (
    spec.type === "donut" &&
    (y.length !== 1 || data.some((r) => (numberValue(r[y[0]]) ?? 0) < 0))
  )
    throw new Error("Donut charts need one nonnegative metric.");
  return {
    version: 1,
    title: spec.title,
    description: spec.description as string | undefined,
    type: spec.type as ChartType,
    x: spec.x,
    y,
    unit: spec.unit as string | undefined,
    data,
  };
}
export function formatValue(value: number, unit?: string) {
  if (unit === "bytes") {
    const units = ["B", "KiB", "MiB", "GiB", "TiB"];
    const index = Math.min(
      4,
      Math.max(0, Math.floor(Math.log2(Math.abs(value) || 1) / 10)),
    );
    return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(value / 1024 ** index)} ${units[index]}`;
  }
  return (
    new Intl.NumberFormat(undefined, { maximumFractionDigits: 3 }).format(
      value,
    ) + (unit ? ` ${unit}` : "")
  );
}
export function csvData(rows: ChartRow[], columns: string[]) {
  const cell = (raw: unknown) => {
    const v =
      typeof raw === "string" && /^[\s]*[=+@-]/.test(raw) ? "'" + raw : raw;
    return `"${String(v ?? "").replaceAll('"', '""')}"`;
  };
  return [
    columns.map(cell).join(","),
    ...rows.map((r) => columns.map((k) => cell(r[k])).join(",")),
  ].join("\r\n");
}
