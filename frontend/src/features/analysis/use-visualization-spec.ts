import { useQuery } from "@tanstack/react-query";
import { privateContentOptions } from "@/shared/private-content";
import { analysisAPI, type ArtifactSpec } from "./api";

export function useVisualizationSpec(
  project: string,
  worker: string,
  id: string,
) {
  return useQuery(visualizationSpecOptions(project, worker, id));
}

export function visualizationSpecOptions(
  project: string,
  worker: string,
  id: string,
) {
  return {
    ...privateContentOptions,
    queryKey: ["project", project, "analysis-artifact", worker, id],
    enabled: !!worker,
    retry: false,
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      analysisAPI(project, worker).spec(id, signal),
    select: (spec: ArtifactSpec) =>
      spec?.schema_version === "1.0" &&
      spec.artifact_id === id &&
      ["table", "chart", "geo"].includes(spec.type)
        ? spec
        : null,
  };
}
