import type { Map as MapLibreMap } from "maplibre-gl";
import type { XY } from "./geo-geometry";

type HeatmapGroup = {
  key: string;
  color: [number, number, number];
  points: { position: XY; weight: number }[];
};

export function installNativeHeatmaps(
  map: MapLibreMap,
  groups: HeatmapGroup[],
  radius: number,
  hidden: Set<string>,
) {
  for (const [index, group] of groups.entries()) {
    const id = `workbench-density-${index}`;
    if (!map.getSource(id)) {
      map.addSource(id, {
        type: "geojson",
        data: {
          type: "FeatureCollection",
          features: group.points.map(({ position, weight }) => ({
            type: "Feature",
            geometry: { type: "Point", coordinates: position },
            properties: { weight },
          })),
        },
      });
      map.addLayer({
        id,
        type: "heatmap",
        source: id,
        layout: { visibility: hidden.has(group.key) ? "none" : "visible" },
        paint: {
          "heatmap-radius": radius,
          "heatmap-weight": ["get", "weight"],
          "heatmap-opacity": 0.8,
          "heatmap-color": [
            "interpolate",
            ["linear"],
            ["heatmap-density"],
            0,
            "rgba(0,0,0,0)",
            0.24,
            `rgba(${group.color.join(",")},0.25)`,
            0.65,
            `rgba(${group.color.join(",")},0.68)`,
            1,
            `rgb(${group.color.join(",")})`,
          ],
        },
      });
    }
  }
  return (next: Set<string>) => {
    for (const [index, group] of groups.entries()) {
      const id = `workbench-density-${index}`;
      if (map.getLayer(id)) {
        map.setLayoutProperty(
          id,
          "visibility",
          next.has(group.key) ? "none" : "visible",
        );
      }
    }
  };
}
