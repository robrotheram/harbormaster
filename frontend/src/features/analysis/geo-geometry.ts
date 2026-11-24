import { GeoJsonLayer, PathLayer, PolygonLayer } from "@deck.gl/layers";
import type { Feature, Geometry } from "geojson";
import type { DataPage, GeoShapeSpec } from "./api";

export type XY = [number, number];
export const latitudeLimit = 85.05112878;

function position(value: unknown): XY | undefined {
  if (!Array.isArray(value)) return;
  const [x, y] = value;
  if (
    value.length === 2 &&
    typeof x === "number" &&
    typeof y === "number" &&
    Math.abs(x) <= 180 &&
    Math.abs(y) <= latitudeLimit
  )
    return [x, y];
}

function list<T>(
  value: unknown,
  parse: (value: unknown) => T | undefined,
  minimum: number,
): T[] | undefined {
  if (!Array.isArray(value) || value.length < minimum) return;
  const parsed = value.map(parse);
  if (parsed.every((entry) => entry !== undefined)) return parsed;
}

const line = (value: unknown) => list(value, position, 2);
function ring(value: unknown) {
  const parsed = list(value, position, 4);
  if (
    parsed &&
    parsed[0][0] === parsed.at(-1)?.[0] &&
    parsed[0][1] === parsed.at(-1)?.[1]
  )
    return parsed;
}
const polygon = (value: unknown) => list(value, ring, 1);

function geometry(input: unknown): Geometry | undefined {
  if (!input || typeof input !== "object" || !("type" in input)) return;
  if (input.type === "Feature" && "geometry" in input) {
    input = input.geometry;
  }
  if (
    !input ||
    typeof input !== "object" ||
    !("type" in input) ||
    !("coordinates" in input)
  )
    return;
  const coordinates = input.coordinates;
  switch (input.type) {
    case "Point": {
      const point = position(coordinates);
      if (point) return { type: "Point", coordinates: point };
      break;
    }
    case "MultiPoint": {
      const points = list(coordinates, position, 1);
      if (points) return { type: "MultiPoint", coordinates: points };
      break;
    }
    case "LineString": {
      const path = line(coordinates);
      if (path) return { type: "LineString", coordinates: path };
      break;
    }
    case "MultiLineString": {
      const paths = list(coordinates, line, 1);
      if (paths) return { type: "MultiLineString", coordinates: paths };
      break;
    }
    case "Polygon": {
      const rings = polygon(coordinates);
      if (rings) return { type: "Polygon", coordinates: rings };
      break;
    }
    case "MultiPolygon": {
      const polygons = list(coordinates, polygon, 1);
      if (polygons) return { type: "MultiPolygon", coordinates: polygons };
      break;
    }
  }
}

function positions(coordinates: unknown): XY[] {
  const point = position(coordinates);
  if (point) return [point];
  return Array.isArray(coordinates) ? coordinates.flatMap(positions) : [];
}

export function geoShapes(page: DataPage, spec: GeoShapeSpec) {
  const column = page.columns.findIndex(
    ({ name }) => name === spec.encoding.shape,
  );
  const paths: XY[][] = [];
  const polygons: XY[][][] = [];
  const features: Feature[] = [];
  const vertices: XY[] = [];
  let excluded = 0;
  for (const row of page.rows) {
    const value = row[column];
    try {
      if (typeof value !== "string") {
        excluded++;
        continue;
      }
      const parsed: unknown = JSON.parse(value);
      if (spec.geometry === "path") {
        const path = line(parsed);
        if (!path || path.length + vertices.length > 20000) {
          excluded++;
          continue;
        }
        paths.push(path);
        vertices.push(...path);
      } else if (spec.geometry === "polygon") {
        const rings = polygon(parsed);
        const points = rings?.flat() || [];
        if (!rings || points.length + vertices.length > 20000) {
          excluded++;
          continue;
        }
        polygons.push(rings);
        vertices.push(...points);
      } else {
        const shape = geometry(parsed);
        const points =
          shape && "coordinates" in shape ? positions(shape.coordinates) : [];
        if (
          !shape ||
          !points.length ||
          points.length + vertices.length > 20000
        ) {
          excluded++;
          continue;
        }
        features.push({ type: "Feature", geometry: shape, properties: {} });
        vertices.push(...points);
      }
    } catch {
      excluded++;
    }
  }
  return { paths, polygons, features, vertices, excluded };
}

export function shapeLayer(
  spec: GeoShapeSpec,
  shapes: ReturnType<typeof geoShapes>,
) {
  switch (spec.geometry) {
    case "path":
      return new PathLayer<XY[]>({
        id: "analysis-paths",
        data: shapes.paths,
        getPath: (path) => path,
        getColor: [30, 112, 143],
        getWidth: 3,
        widthUnits: "pixels",
      });
    case "polygon":
      return new PolygonLayer<XY[][]>({
        id: "analysis-polygons",
        data: shapes.polygons,
        getPolygon: (rings) => rings,
        getFillColor: [30, 112, 143, 90],
        getLineColor: [21, 82, 108],
        lineWidthMinPixels: 1,
      });
    case "geojson":
      return new GeoJsonLayer({
        id: "analysis-geojson",
        data: { type: "FeatureCollection", features: shapes.features },
        getFillColor: [30, 112, 143, 120],
        getLineColor: [21, 82, 108],
        lineWidthMinPixels: 2,
        pointRadiusMinPixels: 4,
      });
  }
}
