import { COORDINATE_SYSTEM, WebMercatorViewport } from "@deck.gl/core";
import {
  ContourLayer,
  GridLayer,
  HeatmapLayer,
  HexagonLayer,
  ScreenGridLayer,
} from "@deck.gl/aggregation-layers";
import type { GeoPointSpec } from "./api";
import type { XY } from "./geo-geometry";

type WeightedPoint = { position: XY; weight: number };

export function contourTooFine(
  spec: GeoPointSpec,
  bounds: {
    left: number;
    right: number;
    bottom: number;
    top: number;
  },
) {
  if (spec.layer !== "contour") return false;
  const center: XY = [
    (bounds.left + bounds.right) / 2,
    (bounds.bottom + bounds.top) / 2,
  ];
  const viewport = new WebMercatorViewport({
    longitude: center[0],
    latitude: center[1],
    zoom: 0,
  });
  const [west, south] = viewport.projectFlat([bounds.left, bounds.bottom]);
  const [east, north] = viewport.projectFlat([bounds.right, bounds.top]);
  const [scaleX, scaleY] = viewport.getDistanceScales(center).unitsPerMeter;
  const cell = spec.cell_size_m ?? 250;
  return (
    (Math.ceil(Math.abs(east - west) / (cell * scaleX)) + 3) *
      (Math.ceil(Math.abs(north - south) / (cell * scaleY)) + 3) >
    200000
  );
}

const contourColors: [number, number, number][] = [
  [22, 114, 119],
  [25, 89, 149],
  [88, 63, 149],
  [156, 57, 112],
  [180, 53, 79],
  [179, 71, 38],
  [137, 82, 18],
  [104, 51, 33],
];

export function aggregateLayer(
  spec: GeoPointSpec,
  points: WeightedPoint[],
  group?: { key: string; color: [number, number, number] },
) {
  const common = {
    data: points,
    coordinateSystem: COORDINATE_SYSTEM.LNGLAT,
    getPosition: (point: WeightedPoint) => point.position,
    getWeight: (point: WeightedPoint) => point.weight,
  };
  switch (spec.layer) {
    case "grid":
      return new GridLayer<WeightedPoint>({
        ...common,
        id: "analysis-grid",
        cellSize: spec.cell_size_m ?? 250,
        colorAggregation: "SUM",
        getColorWeight: common.getWeight,
        gpuAggregation: false,
        extruded: false,
        opacity: 0.75,
      });
    case "hexagon":
      return new HexagonLayer<WeightedPoint>({
        ...common,
        id: "analysis-hexagon",
        radius: spec.radius_m ?? 250,
        colorAggregation: "SUM",
        getColorWeight: common.getWeight,
        gpuAggregation: false,
        extruded: false,
        opacity: 0.75,
      });
    case "contour":
      return new ContourLayer<WeightedPoint>({
        ...common,
        id: "analysis-contour",
        cellSize: spec.cell_size_m ?? 250,
        aggregation: "SUM",
        gpuAggregation: false,
        contours: [...(spec.thresholds ?? [1, 5, 10])]
          .sort((a, b) => a - b)
          .map((threshold, index) => ({
            threshold,
            color: contourColors[index],
            strokeWidth: 2,
          })),
      });
    case "screen_grid":
      return new ScreenGridLayer<WeightedPoint>({
        ...common,
        id: "analysis-screen-grid",
        cellSizePixels: spec.cell_size_px ?? 32,
        aggregation: "SUM",
        opacity: 0.65,
      });
    default:
      return new HeatmapLayer<WeightedPoint>({
        ...common,
        id: group?.key ?? "analysis-heatmap",
        ...(group && {
          colorRange: [0.15, 0.32, 0.53, 0.74, 0.9, 1].map(
            (strength, index) =>
              [
                ...group.color.map((channel) =>
                  Math.round(255 + (channel - 255) * strength),
                ),
                Math.round((255 * (index + 1)) / 6),
              ] as [number, number, number, number],
          ),
        }),
        radiusPixels: spec.radius_px ?? 35,
        aggregation: "SUM",
        weightsTextureSize: 512,
        opacity: 0.75,
      });
  }
}
