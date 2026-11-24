import { analysisAPI } from "./api";
import {
  reportDataNote,
  type ReportPart,
  type ReportSource,
} from "./report-model";

export async function prepareReportParts({
  project,
  items,
  signal,
}: {
  project: string;
  items: ReportSource[];
  signal: AbortSignal;
}) {
  const parts: ReportPart[] = [];
  for (const item of items) {
    signal.throwIfAborted();
    const { worker_id: worker, artifact_id: id } = item;
    const title = item.title ?? id;
    const api = analysisAPI(project, worker);
    const part: ReportPart = {
      item: { ...item, title },
    };
    let loaded = false;
    try {
      const spec = await api.spec(id, signal);
      if (
        spec.schema_version !== "1.0" ||
        spec.artifact_id !== id ||
        !["table", "chart", "geo"].includes(spec.type) ||
        (item.type !== undefined && spec.type !== item.type)
      )
        throw new Error("Invalid artifact");
      part.item = { ...item, title: item.title ?? spec.title, type: spec.type };
      const page = await api.data(
        id,
        spec.type === "table" ? { limit: 25 } : {},
        signal,
      );
      loaded = true;
      part.note = reportDataNote(page, spec.type === "table");
      if (spec.type === "table") {
        part.table = page;
      } else if (spec.type === "chart") {
        const { chartOptions } = await import("./analysis-chart");
        const { chartSVG } = await import("@/features/work/chart-engine");
        const options =
          page.rows.length && chartOptions(spec.visualization, page);
        if (options)
          part.figure = {
            src: "data:image/svg+xml," + encodeURIComponent(chartSVG(options)),
          };
      } else {
        const { captureReportMap } = await import("./report-map");
        part.figure =
          (await captureReportMap(spec.visualization, page, signal)) ??
          undefined;
      }
    } catch {
      signal.throwIfAborted();
      part.unavailable = true;
      if (!loaded) part.note = "Data is unavailable on the originating worker.";
    }
    if (loaded && part.item.type !== "table" && !part.figure) {
      part.unavailable = true;
      part.note = [
        part.note,
        "A static preview could not be created for this visualization.",
      ]
        .filter(Boolean)
        .join(" ");
    }
    parts.push(part);
  }
  signal.throwIfAborted();
  return parts;
}
