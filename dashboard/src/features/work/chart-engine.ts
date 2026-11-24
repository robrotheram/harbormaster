import { init, use, type EChartsCoreOption } from "echarts/core";
import { BarChart, LineChart, ScatterChart, PieChart } from "echarts/charts";
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  AriaComponent,
} from "echarts/components";
import { SVGRenderer } from "echarts/renderers";
use([
  BarChart,
  LineChart,
  ScatterChart,
  PieChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  AriaComponent,
  SVGRenderer,
]);
export const createChart = (element: HTMLElement) =>
  init(element, undefined, { renderer: "svg" });

export function chartSVG(options: EChartsCoreOption) {
  const chart = init(null, undefined, {
    renderer: "svg",
    ssr: true,
    width: 880,
    height: 360,
  });
  try {
    chart.setOption(options);
    return chart.renderToSVGString();
  } finally {
    chart.dispose();
  }
}
