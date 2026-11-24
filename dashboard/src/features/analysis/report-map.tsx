import { createRoot } from "react-dom/client";
import type { DataPage, GeoSpec } from "./api";
import GeoPoints from "./geo-points";
import type { ReportFigure } from "./report-model";

export async function captureReportMap(
  spec: GeoSpec,
  page: DataPage,
  signal: AbortSignal,
): Promise<ReportFigure | null> {
  signal.throwIfAborted();
  const node = document.createElement("div");
  node.className = "pointer-events-none fixed top-0 -left-[10000px] w-[880px]";
  node.inert = true;
  node.setAttribute("aria-hidden", "true");
  document.body.append(node);
  const root = createRoot(node);
  try {
    return await new Promise<ReportFigure | null>((resolve) => {
      const abort = () => finish(null);
      const timeout = window.setTimeout(abort, 15000);
      function finish(figure: ReportFigure | null) {
        window.clearTimeout(timeout);
        signal.removeEventListener("abort", abort);
        resolve(figure);
      }
      signal.addEventListener("abort", abort, { once: true });
      root.render(<GeoPoints spec={spec} page={page} onSnapshot={finish} />);
    });
  } finally {
    root.unmount();
    node.remove();
  }
}
