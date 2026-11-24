import { expect, test } from "vitest";
import { geoShapes, shapeLayer } from "./geo-geometry";
import { aggregateLayer, contourTooFine } from "./geo-aggregation";
import type { DataPage, GeoPointSpec, GeoShapeSpec } from "./api";

const geographic = {
  coordinate_system: { type: "geographic", crs: "EPSG:4326" },
  view: { mode: "map" },
} as const;

test.each([
  [
    "path",
    [
      [0, 0],
      [1, 1],
    ],
    "PathLayer",
  ],
  [
    "polygon",
    [
      [
        [0, 0],
        [1, 0],
        [1, 1],
        [0, 0],
      ],
    ],
    "PolygonLayer",
  ],
  [
    "geojson",
    {
      type: "Feature",
      geometry: { type: "Point", coordinates: [1, 1] },
      properties: { markup: "<b>ignore</b>" },
    },
    "GeoJsonLayer",
  ],
] as const)(
  "validates %s data and selects its deck.gl layer",
  (geometry, input, name) => {
    const spec: GeoShapeSpec = {
      ...geographic,
      geometry,
      encoding: { shape: "shape" },
    };
    const data: DataPage = {
      columns: [{ name: "shape", type: "VARCHAR" }],
      rows: [[JSON.stringify(input)], ["["], ["[181,0]"]],
      next_cursor: null,
      has_more: false,
      truncated: false,
      sampled: false,
    };
    const parsed = geoShapes(data, spec);
    expect(parsed.excluded).toBe(2);
    expect(parsed.vertices.length).toBeGreaterThan(0);
    expect(shapeLayer(spec, parsed).constructor.name).toBe(name);
    if (geometry === "geojson")
      expect(parsed.features[0].properties).toEqual({});
  },
);

test.each([
  ["grid", { cell_size_m: 100 }, "GridLayer", "cellSize", 100],
  ["hexagon", { radius_m: 150 }, "HexagonLayer", "radius", 150],
  ["contour", { cell_size_m: 200 }, "ContourLayer", "cellSize", 200],
  [
    "screen_grid",
    { cell_size_px: 24 },
    "ScreenGridLayer",
    "cellSizePixels",
    24,
  ],
] as const)(
  "selects %s and translates its public sizing option",
  (layer, settings, name, key, size) => {
    const spec: GeoPointSpec = {
      ...geographic,
      ...settings,
      geometry: "point",
      layer,
      encoding: { x: "x", y: "y" },
    };
    const renderer = aggregateLayer(spec, [{ position: [1, 2], weight: 3 }]);
    expect(renderer.constructor).toHaveProperty("layerName", name);
    expect(renderer.props).toMatchObject({ [key]: size });
  },
);

test("rejects a contour cell size that would create an oversized grid", () => {
  const spec: GeoPointSpec = {
    ...geographic,
    geometry: "point",
    layer: "contour",
    cell_size_m: 50,
    encoding: { x: "x", y: "y" },
  };
  expect(
    contourTooFine(spec, { left: -179, right: 179, bottom: -85, top: 85 }),
  ).toBe(true);
  expect(
    contourTooFine(spec, {
      left: -0.12,
      right: -0.11,
      bottom: 51.5,
      top: 51.51,
    }),
  ).toBe(false);
});
