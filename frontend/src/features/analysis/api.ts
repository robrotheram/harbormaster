import { request, uploadFile } from "@/shared/http";

export type Column = { name: string; type: string };
export type Cell = string | number | boolean | null;
export type Dataset = {
  dataset_id: string;
  name: string;
  format: "csv" | "parquet";
  size_bytes: number;
  sha256: string;
};
export type DataPage = {
  columns: Column[];
  rows: Cell[][];
  next_cursor: string | null;
  has_more: boolean;
  truncated: boolean;
  sampled: boolean;
};

type Direction = "asc" | "desc";
type FilterOp = "contains" | "eq" | "is_null" | "is_not_null";
export type PreviewRequest = {
  limit?: number;
  cursor?: string;
  sort?: { field: string; direction: Direction }[];
  filters?: { field: string; op: FilterOp; value?: string }[];
};

type Envelope = {
  schema_version: "1.0";
  artifact_id: string;
  title: string;
  description?: string;
  data: { source_type: "dataset" | "query_result"; reference: string };
};
export type Axis = {
  field: string;
  type: "temporal" | "quantitative" | "nominal";
  label?: string;
};
export type TableSpec = {
  page_size?: number;
  sortable?: boolean;
  filterable?: boolean;
  columns?: { field: string }[];
};
type ChartColors = Record<string, string>;
type CartesianChart = {
  chart_type: "line" | "bar" | "scatter" | "histogram";
  encoding: { x: Axis; y: Axis; series?: string };
  style?: {
    category_colors?: ChartColors;
    series_colors?: ChartColors;
    gridlines?: { x?: boolean; y?: boolean };
  };
};
type PieChart = {
  chart_type: "pie";
  encoding: { category: string; value: string };
  style?: { category_colors?: ChartColors };
};
export type ChartSpec = CartesianChart | PieChart;
type GeoView = {
  coordinate_system: {
    type: "geographic" | "projected" | "cartesian";
    crs?: string;
  };
  view: { mode: "map" | "cartesian"; auto_fit?: boolean };
};
export type GeoPointSpec = GeoView & {
  geometry: "point";
  layer?: "points" | "heatmap" | "grid" | "hexagon" | "contour" | "screen_grid";
  radius_px?: number;
  cell_size_m?: number;
  radius_m?: number;
  cell_size_px?: number;
  thresholds?: number[];
  encoding: {
    x: string;
    y: string;
    color?: string;
    weight?: string;
    group?: string;
  };
};
export type GeoShapeSpec = GeoView & {
  geometry: "path" | "polygon" | "geojson";
  encoding: { shape: string };
};
export type GeoSpec = GeoPointSpec | GeoShapeSpec;
export type ArtifactSpec = Envelope &
  (
    | { type: "table"; visualization: TableSpec }
    | { type: "chart"; visualization: ChartSpec }
    | { type: "geo"; visualization: GeoSpec }
  );

export type VisualizationReference = {
  worker_id: string;
  artifact_id: string;
};

export type VisualizationSummary = {
  artifact_id: string;
  title: string;
  type: ArtifactSpec["type"];
};

export type VisualizationRevision = VisualizationReference & {
  title: string;
  report_paths?: string[];
};
export type VisualizationDetails = VisualizationReference & {
  title?: string;
  description?: string;
};
export function visualizationDetailsAPI(project: string) {
  const path = `/projects/${encodeURIComponent(project)}/analysis/visualization-details`;
  return {
    list: (signal?: AbortSignal) =>
      request<{ titles: VisualizationDetails[] }>(path, undefined, signal),
    update: (input: VisualizationDetails) =>
      request<VisualizationDetails>(path, input),
  };
}

export function artifactReference(value: string): string | undefined {
  const id = value.trim();
  return /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$/.test(id) ? id : undefined;
}

export function visualizationReference(
  value: string,
  defaultWorker: string,
): VisualizationReference | undefined {
  const parts = value.trim().split("/");
  if (parts.length > 2) return;
  const [worker, artifact] =
    parts.length === 2 ? parts : [defaultWorker, parts[0]];
  const worker_id = parts.length === 2 ? artifactReference(worker) : worker;
  const artifact_id = artifactReference(artifact);
  if (worker_id !== undefined && artifact_id) return { worker_id, artifact_id };
}

export function analysisAPI(project: string, worker: string) {
  function path(tail: string, name?: string) {
    const query = new URLSearchParams({ project_id: project });
    if (name) query.set("name", name);
    return `/workspaces/${encodeURIComponent(worker)}/analysis${tail}?${query}`;
  }
  return {
    list: (signal?: AbortSignal) =>
      request<{ items: Dataset[] }>(path("/datasets"), undefined, signal),
    upload: (file: File, signal: AbortSignal) =>
      uploadFile<Dataset>(path("/datasets/upload", file.name), file, signal),
    schema: (id: string, signal?: AbortSignal) =>
      request<Dataset & { columns: Column[] }>(
        path(`/datasets/${encodeURIComponent(id)}/schema`),
        undefined,
        signal,
      ),
    preview: (id: string, query: PreviewRequest, signal?: AbortSignal) =>
      request<DataPage>(
        path(`/datasets/${encodeURIComponent(id)}/preview`),
        query,
        signal,
      ),
    artifacts: (signal?: AbortSignal) =>
      request<{ items: VisualizationSummary[] }>(
        path("/artifacts"),
        undefined,
        signal,
      ),
    deleteArtifact: (id: string) =>
      request<{ deleted: boolean }>(
        path(`/artifacts/${encodeURIComponent(id)}/delete`),
        {},
      ),
    spec: (id: string, signal?: AbortSignal) =>
      request<ArtifactSpec>(
        path(`/artifacts/${encodeURIComponent(id)}`),
        undefined,
        signal,
      ),
    data: (id: string, query: PreviewRequest, signal?: AbortSignal) =>
      request<DataPage>(
        path(`/artifacts/${encodeURIComponent(id)}/data`),
        query,
        signal,
      ),
  };
}

export function finite(value: Cell | undefined): number | null {
  if (typeof value !== "number" && (typeof value !== "string" || !value.trim()))
    return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}
