import { useQuery } from "@tanstack/react-query";
import { privateContentOptions } from "@/shared/private-content";
import { analysisAPI } from "./api";

export function workerDatasetsKey(project: string, worker: string) {
  return ["project", project, "analysis-datasets", worker] as const;
}

export function useWorkerDatasets(
  project: string,
  worker: string,
  ready: boolean,
) {
  return useQuery({
    ...privateContentOptions,
    queryKey: workerDatasetsKey(project, worker),
    queryFn: ({ signal }) => analysisAPI(project, worker).list(signal),
    enabled: ready && !!worker,
    retry: false,
  });
}
