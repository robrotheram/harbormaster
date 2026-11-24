import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useUserSession } from "@/features/auth/session";
import { privateContentOptions } from "@/shared/private-content";
import { visualizationDetailsAPI, type VisualizationDetails } from "./api";

function useVisualizationDetailsKey(project: string) {
  const session = useUserSession();
  const owner =
    session.status === "authenticated" ? session.user.id : session.status;
  return ["project", project, "visualization-details", session.status, owner];
}

export function useVisualizationDetails(project: string) {
  const queryKey = useVisualizationDetailsKey(project);
  return useQuery({
    ...privateContentOptions,
    queryKey,
    staleTime: 10000,
    retry: false,
    queryFn: ({ signal }) => visualizationDetailsAPI(project).list(signal),
  });
}

export function useUpdatePersonalVisualization(project: string) {
  const client = useQueryClient();
  const queryKey = useVisualizationDetailsKey(project);
  return useMutation({
    mutationFn: (input: VisualizationDetails) =>
      visualizationDetailsAPI(project).update(input),
    onSuccess: () => client.invalidateQueries({ queryKey }),
  });
}
