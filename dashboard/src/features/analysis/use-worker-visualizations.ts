import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { privateContentOptions } from "@/shared/private-content";
import { analysisAPI } from "./api";

export function catalogKey(project: string, worker: string) {
  return ["project", project, "analysis-artifacts", worker] as const;
}

export function useWorkerVisualizations(
  project: string,
  worker: string,
  enabled = true,
) {
  return useQuery({
    ...privateContentOptions,
    queryKey: catalogKey(project, worker),
    enabled: !!worker && enabled,
    retry: false,
    queryFn: ({ signal }) => analysisAPI(project, worker).artifacts(signal),
  });
}

export function useDeleteVisualization(project: string, worker: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => analysisAPI(project, worker).deleteArtifact(id),
    onSuccess: (_, id) =>
      Promise.all([
        client.invalidateQueries({ queryKey: catalogKey(project, worker) }),
        client.invalidateQueries({
          queryKey: ["project", project, "analysis-artifact", worker, id],
        }),
        client.invalidateQueries({
          queryKey: ["project", project, "analysis-visual-data", worker, id],
        }),
        client.invalidateQueries({
          queryKey: ["project", project, "report-visuals"],
        }),
      ]),
  });
}
