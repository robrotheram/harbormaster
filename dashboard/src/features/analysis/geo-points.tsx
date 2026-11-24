import { useEffect, useMemo, useRef, useState } from "react";
import {
  COORDINATE_SYSTEM,
  Deck,
  OrthographicView,
  type PickingInfo,
} from "@deck.gl/core";
import { ScatterplotLayer } from "@deck.gl/layers";
import { MapLibreOverlay } from "@deck.gl/maplibre";
import {
  Map as MapLibreMap,
  NavigationControl,
  setWorkerUrl,
  type StyleSpecification,
} from "maplibre-gl";
import maplibreWorker from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url";
import "maplibre-gl/dist/maplibre-gl.css";
import landAtlas from "world-atlas/land-110m.json";
import { feature } from "topojson-client";
import type { GeometryCollection, Topology } from "topojson-specification";
import {
  finite,
  type Cell,
  type DataPage,
  type GeoSpec,
  type GeoPointSpec,
} from "./api";

import { aggregateLayer, contourTooFine } from "./geo-aggregation";
import { geoShapes, latitudeLimit, shapeLayer, type XY } from "./geo-geometry";

import { installNativeHeatmaps } from "./geo-heatmap-fallback";
import type { ReportFigure } from "./report-model";

setWorkerUrl(maplibreWorker);

const land = landAtlas as unknown as Topology<{ land: GeometryCollection }>;
const landFeatures = feature(land, land.objects.land);
const detailedStyle = "https://tiles.openfreemap.org/styles/positron";
const mapStyle: StyleSpecification = {
  version: 8,
  sources: {
    land: { type: "geojson", data: landFeatures, attribution: "Natural Earth" },
  },
  layers: [
    {
      id: "water",
      type: "background",
      paint: { "background-color": "#dce8ed" },
    },
    {
      id: "land",
      type: "fill",
      source: "land",
      paint: { "fill-color": "#f2ede1", "fill-outline-color": "#a9b6ae" },
    },
  ],
};

type Point = {
  position: [number, number];
  row: Cell[];
  value?: Cell;
  group?: Cell;
  weight: number;
};
type RGB = [number, number, number];
const palette: RGB[] = [
  [10, 111, 126],
  [146, 75, 124],
  [209, 101, 59],
  [52, 83, 156],
  [75, 122, 65],
  [192, 150, 26],
  [129, 85, 54],
  [122, 88, 181],
  [175, 55, 79],
  [71, 125, 168],
  [85, 100, 103],
  [103, 127, 28],
];

export function geoPoints(page: DataPage, spec: GeoPointSpec): Point[] {
  const { x, y, color, weight, group } = spec.encoding;
  const names = page.columns.map(({ name }) => name);
  const colorIndex = color ? names.indexOf(color) : -1;
  const colorType = page.columns[colorIndex]?.type;
  const category =
    colorType === "VARCHAR" ||
    colorType === "BOOLEAN" ||
    colorType?.startsWith("ENUM(");
  const groupIndex = group ? names.indexOf(group) : category ? colorIndex : -1;
  const weightIndex = weight ? names.indexOf(weight) : -1;
  const xi = names.indexOf(x);
  const yi = names.indexOf(y);
  return page.rows.flatMap((row) => {
    const px = finite(row[xi]);
    const py = finite(row[yi]);
    if (px === null || py === null) return [];
    if (
      spec.view.mode === "map" &&
      (Math.abs(px) > 180 || Math.abs(py) > latitudeLimit)
    )
      return [];
    const pointWeight = weight ? finite(row[weightIndex]) : 1;
    if (pointWeight === null || pointWeight <= 0) return [];
    return [
      {
        position: [px, py],
        row,
        value: row[colorIndex],
        group: groupIndex >= 0 ? row[groupIndex] : undefined,
        weight: pointWeight,
      },
    ];
  });
}

function pointGroups(points: Point[]) {
  const groups = new Map<
    string,
    { key: string; label: string; points: Point[] }
  >();
  for (const point of points) {
    if (point.group === undefined) continue;
    const key = JSON.stringify(point.group);
    const group = groups.get(key) ?? {
      key,
      label: String(point.group ?? "Missing value"),
      points: [],
    };
    group.points.push(point);
    groups.set(key, group);
  }
  const ranked = [...groups.values()].sort(
    (a, b) =>
      b.points.length - a.points.length || a.label.localeCompare(b.label),
  );
  if (ranked.length > palette.length) {
    const other = ranked.splice(palette.length - 1);
    ranked.push({
      key: "other",
      label: "Other categories",
      points: other.flatMap((group) => group.points),
    });
  }
  return ranked.map((group, index) => ({ ...group, color: palette[index] }));
}

function pointColor(point: Point, min: number, max: number): RGB {
  if (point.value == null) return palette[0];
  const value = finite(point.value);
  if (value !== null) {
    const t = max > min ? (value - min) / (max - min) : 0.5;
    return [
      Math.round(25 + t * 205),
      Math.round(140 - t * 60),
      Math.round(160 - t * 118),
    ];
  }
  let hash = 0;
  for (const ch of String(point.value))
    hash = (Math.imul(hash, 31) + ch.charCodeAt(0)) >>> 0;
  return palette[hash % palette.length];
}

function pointBounds(points: XY[]) {
  const x = points.map(([x]) => x);
  const y = points.map(([, y]) => y);
  return {
    left: Math.min(...x),
    right: Math.max(...x),
    bottom: Math.min(...y),
    top: Math.max(...y),
  };
}

export default function GeoPoints({
  spec,
  page,
  onSnapshot,
}: {
  spec: GeoSpec;
  page: DataPage;
  onSnapshot?: (figure: ReportFigure | null) => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const [tilesUnavailable, setTilesUnavailable] = useState(false);
  const [picked, setPicked] = useState<Point | null>(null);
  const [error, setError] = useState(false);
  const [hiddenGroups, setHiddenGroups] = useState<Set<string>>(
    () => new Set(),
  );
  const visibility = useRef(hiddenGroups);
  const applyVisibility = useRef<((hidden: Set<string>) => void) | undefined>(
    undefined,
  );

  function toggleGroup(key: string) {
    const next = new Set(visibility.current);
    if (!next.delete(key)) next.add(key);
    visibility.current = next;
    setHiddenGroups(next);
    setPicked(null);
    applyVisibility.current?.(next);
  }
  const points = useMemo(
    () => (spec.geometry === "point" ? geoPoints(page, spec) : []),
    [page, spec],
  );
  const groups = useMemo(() => pointGroups(points), [points]);
  const shapes = useMemo(
    () => (spec.geometry === "point" ? undefined : geoShapes(page, spec)),
    [page, spec],
  );
  const bounds = useMemo(
    () =>
      pointBounds(shapes?.vertices ?? points.map(({ position }) => position)),
    [shapes, points],
  );
  const isMap = spec.view.mode === "map";
  const pointSpec = spec.geometry === "point" ? spec : undefined;
  const aggregation =
    pointSpec?.layer && pointSpec.layer !== "points"
      ? pointSpec.layer
      : undefined;
  const hasGeometry = Boolean(shapes ? shapes.vertices.length : points.length);
  const contourLimit = Boolean(
    hasGeometry && pointSpec && contourTooFine(pointSpec, bounds),
  );
  useEffect(() => {
    if (error || !hasGeometry || contourLimit) onSnapshot?.(null);
  }, [error, hasGeometry, contourLimit, onSnapshot]);
  useEffect(() => {
    const node = host.current;
    if (!node || !hasGeometry || contourLimit) return;
    const numeric = points
      .map(({ value }) => finite(value))
      .filter((value) => value !== null);
    const min = Math.min(...numeric);
    const max = Math.max(...numeric);
    const title = (info: PickingInfo<Point>) =>
      pointSpec && info.object
        ? {
            text: `${pointSpec.encoding.x} ${info.object.position[0]}\n${pointSpec.encoding.y} ${info.object.position[1]}`,
          }
        : null;
    function scatter(data: Point[], id: string, color?: RGB) {
      return new ScatterplotLayer<Point>({
        id,
        data,
        coordinateSystem: isMap
          ? COORDINATE_SYSTEM.LNGLAT
          : COORDINATE_SYSTEM.CARTESIAN,
        getPosition: (point) => point.position,
        getFillColor: color ?? ((point: Point) => pointColor(point, min, max)),
        getRadius: 4,
        radiusUnits: "pixels",
        pickable: true,
        opacity: 0.8,
        onClick: ({ object }) => {
          if (object) setPicked(object);
        },
      });
    }
    const layers =
      spec.geometry !== "point" && shapes
        ? [shapeLayer(spec, shapes)]
        : pointSpec && aggregation
          ? aggregation === "heatmap" && groups.length
            ? groups.map((group) =>
                aggregateLayer(pointSpec, group.points, group),
              )
            : [aggregateLayer(pointSpec, points)]
          : groups.length
            ? groups.map((group) =>
                scatter(group.points, group.key, group.color),
              )
            : [scatter(points, "analysis-points")];
    const visibleLayers = (hidden: Set<string>) =>
      layers.filter((layer) => !hidden.has(layer.id));
    const autoFit = !!onSnapshot || spec.view.auto_fit !== false;
    let captured = false;
    function capture(canvas: HTMLCanvasElement | null, attribution?: string) {
      if (!onSnapshot || captured) return;
      captured = true;
      try {
        const src = canvas?.toDataURL("image/png");
        onSnapshot(
          src?.startsWith("data:image/png;")
            ? {
                src,
                attribution,
                legend: groups.map(({ label, color }) => ({
                  label,
                  color: `rgb(${color.join(",")})`,
                })),
                density: aggregation === "heatmap" && groups.length === 0,
              }
            : null,
        );
      } catch {
        onSnapshot(null);
      }
    }
    let map: MapLibreMap | undefined;
    let deck: Deck<OrthographicView> | undefined;
    let resize: ResizeObserver | undefined;
    let fallbackFrame: number | undefined;
    try {
      if (isMap) {
        const geographic = new MapLibreMap({
          container: node,
          style: detailedStyle,
          center: [0, 0],
          zoom: 0,
          renderWorldCopies: false,
          ...(onSnapshot && {
            interactive: false,
            pixelRatio: 1,
            canvasContextAttributes: { preserveDrawingBuffer: true },
          }),
        });
        map = geographic;
        let offline = false;
        geographic.on("error", () => {
          if (!offline) {
            offline = true;
            geographic.setStyle(mapStyle, { diff: false });
            setTilesUnavailable(true);
          } else {
            setError(true);
          }
        });
        if (!onSnapshot)
          geographic.addControl(new NavigationControl({ showCompass: false }));
        geographic.once("load", () => {
          setTilesUnavailable(offline);
          let overlay: MapLibreOverlay | undefined;
          let usingNativeHeatmap = false;
          let nativeHeatmapInstalled = false;
          function installFallback() {
            try {
              const heatmapGroups = groups.length
                ? groups
                : [
                    {
                      key: "analysis-heatmap",
                      points,
                      color: [194, 40, 23] as RGB,
                    },
                  ];
              applyVisibility.current = installNativeHeatmaps(
                geographic,
                heatmapGroups,
                pointSpec?.radius_px ?? 35,
                visibility.current,
              );
              nativeHeatmapInstalled = true;
            } catch {
              setError(true);
            }
          }
          function useNativeHeatmap() {
            if (usingNativeHeatmap) return;
            usingNativeHeatmap = true;
            fallbackFrame = requestAnimationFrame(() => {
              if (overlay && geographic.hasControl(overlay))
                geographic.removeControl(overlay);
              if (geographic.isStyleLoaded()) installFallback();
              else geographic.once("idle", installFallback);
              geographic.on("style.load", installFallback);
            });
          }
          const gl = geographic.getCanvas().getContext("webgl2");
          if (
            aggregation === "heatmap" &&
            (!gl?.getExtension("EXT_float_blend") ||
              !gl.getExtension("EXT_color_buffer_float"))
          ) {
            useNativeHeatmap();
          } else {
            overlay = new MapLibreOverlay({
              layers: visibleLayers(visibility.current),
              getTooltip: pointSpec && !aggregation ? title : undefined,
              onError:
                aggregation === "heatmap"
                  ? useNativeHeatmap
                  : () => setError(true),
              interleaved: true,
            });
            try {
              geographic.addControl(overlay);
              applyVisibility.current = (hidden) =>
                overlay?.setProps({ layers: visibleLayers(hidden) });
            } catch {
              if (aggregation === "heatmap") useNativeHeatmap();
              else setError(true);
            }
          }
          if (autoFit)
            geographic.fitBounds(
              [
                [bounds.left, bounds.bottom],
                [bounds.right, bounds.top],
              ],
              { padding: 30, maxZoom: 15, duration: 0 },
            );
          if (onSnapshot) {
            geographic.on("idle", () => {
              if (usingNativeHeatmap && !nativeHeatmapInstalled) return;
              const attribution = node
                .querySelector(".maplibregl-ctrl-attrib-inner")
                ?.textContent?.replace(/\s+/g, " ")
                .trim();
              capture(geographic.getCanvas(), attribution);
            });
          }
        });
        resize = new ResizeObserver(() => geographic.resize());
        resize.observe(node);
      } else {
        const scale = Math.min(
          node.clientWidth / Math.max(bounds.right - bounds.left, 1),
          node.clientHeight / Math.max(bounds.top - bounds.bottom, 1),
        );
        deck = new Deck({
          parent: node,
          width: "100%",
          height: "100%",
          views: new OrthographicView({ id: "xy", flipY: false }),
          controller: !onSnapshot,
          initialViewState: {
            target: autoFit
              ? [
                  (bounds.left + bounds.right) / 2,
                  (bounds.bottom + bounds.top) / 2,
                  0,
                ]
              : [0, 0, 0],
            zoom: autoFit ? Math.log2(Math.max(scale * 0.82, 1e-20)) : 0,
            minZoom: -64,
            maxZoom: 32,
          },
          layers: visibleLayers(visibility.current),
          getTooltip: title,
          onError: () => setError(true),
          ...(onSnapshot && {
            useDevicePixels: 1,
            onAfterRender: () => {
              if (deck) capture(deck.getCanvas());
            },
          }),
        });
        applyVisibility.current = (hidden) =>
          deck?.setProps({ layers: visibleLayers(hidden) });
      }
    } catch {
      setError(true);
    }
    return () => {
      if (fallbackFrame !== undefined) cancelAnimationFrame(fallbackFrame);
      applyVisibility.current = undefined;
      resize?.disconnect();
      map?.remove();
      deck?.finalize();
    };
  }, [
    points,
    groups,
    isMap,
    pointSpec,
    aggregation,
    spec,
    shapes,
    bounds,
    hasGeometry,
    contourLimit,
    onSnapshot,
  ]);
  if (error)
    return (
      <p role="status">
        The geographic renderer could not draw this layer. Check WebGL support
        and reload.
      </p>
    );
  if (contourLimit)
    return (
      <p role="status">
        The contour grid is too fine for this extent. Increase the cell size or
        restrict the data to a smaller area.
      </p>
    );
  if (!hasGeometry)
    return (
      <p role="status">
        {shapes
          ? "No valid geographic shapes are available."
          : "No valid XY points are available."}
      </p>
    );
  const layerLabel = aggregation
    ? aggregation.replace("_", " ")
    : shapes
      ? spec.geometry
      : "points";
  const rowExclusions = shapes?.excluded ?? page.rows.length - points.length;
  const details =
    aggregation === "heatmap"
      ? []
      : [
          pointSpec
            ? `${pointSpec.encoding.x} × ${pointSpec.encoding.y}`
            : layerLabel,
          aggregation && layerLabel,
          spec.coordinate_system.crs,
          groups.length === 0 &&
            pointSpec?.encoding.color &&
            `Color by ${pointSpec.encoding.color}`,
        ].filter(Boolean);
  if (pointSpec?.encoding.weight)
    details.push(`Weighted by ${pointSpec.encoding.weight}`);
  if (rowExclusions > 0)
    details.push(`${rowExclusions} rows excluded by geometry or weight limits`);
  return (
    <div
      className="space-y-2 text-sm"
      data-analysis-plot-group={onSnapshot ? undefined : true}
    >
      {isMap && tilesUnavailable && (
        <p role="status">
          Detailed tiles are unavailable. Showing the offline map.
        </p>
      )}
      {(!aggregation || aggregation === "heatmap") && groups.length > 0 && (
        <fieldset className="flex min-w-0 flex-wrap gap-x-4 gap-y-2 rounded-md border px-3 pb-3 pt-2">
          <legend className="px-1 font-medium">Map layers</legend>
          {groups.map((group) => (
            <label
              key={group.key}
              className="inline-flex min-w-0 items-center gap-2"
            >
              <input
                type="checkbox"
                checked={!hiddenGroups.has(group.key)}
                onChange={() => toggleGroup(group.key)}
              />
              <span
                aria-hidden="true"
                className="size-3 shrink-0 rounded-full"
                style={{ backgroundColor: `rgb(${group.color.join(",")})` }}
              />
              <span className="break-all">{group.label}</span>
              <span aria-hidden="true" className="text-muted-foreground">
                {group.points.length.toLocaleString()}
              </span>
            </label>
          ))}
        </fieldset>
      )}
      <div
        ref={host}
        data-analysis-plot={onSnapshot ? undefined : true}
        className={
          onSnapshot
            ? "relative h-[420px] w-full overflow-hidden"
            : "relative h-96 max-h-[65vh] min-h-56 overflow-hidden rounded-md border bg-muted/30"
        }
        aria-label={isMap ? `Geographic ${layerLabel}` : "Cartesian points"}
      />
      {aggregation === "heatmap" && groups.length === 0 && (
        <div className="flex flex-wrap items-center gap-2 text-muted-foreground">
          <span>Lower density</span>
          <span
            className="h-2 w-24 rounded-sm bg-linear-to-r from-[#ffffb2] via-[#fd8d3c] to-[#bd0026]"
            aria-hidden="true"
          />
          <span>Higher density</span>
        </div>
      )}
      {details.length > 0 && (
        <p className="text-muted-foreground">{details.join(" · ")}</p>
      )}
      {picked && (
        <dl
          className="grid grid-cols-[auto_1fr] gap-x-3 rounded-md border p-3"
          aria-label="Selected point"
        >
          {page.columns.map(({ name }, i) => (
            <div className="contents" key={name}>
              <dt className="font-medium">{name}</dt>
              <dd className="min-w-0 [overflow-wrap:anywhere]">
                {String(picked.row[i] ?? "null")}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  );
}
