import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useUserSession } from "@/features/auth/session";
import { privateContentOptions } from "@/shared/private-content";
import {
  savedVisualizationsAPI,
  type PersonalVisualizationUpdate,
} from "./api";

function useSavedVisualizationsKey(project: string) {
  const session = useUserSession();
  const owner =
    session.status === "authenticated" ? session.user.id : session.status;
  return ["project", project, "saved-visualizations", session.status, owner];
}

export function useSavedVisualizations(project: string) {
  const queryKey = useSavedVisualizationsKey(project);
  return useQuery({
    ...privateContentOptions,
    queryKey,
    staleTime: 10000,
    retry: false,
    queryFn: ({ signal }) => savedVisualizationsAPI(project).list(signal),
  });
}

export function useUpdatePersonalVisualization(project: string) {
  const client = useQueryClient();
  const queryKey = useSavedVisualizationsKey(project);
  return useMutation({
    mutationFn: (input: PersonalVisualizationUpdate) =>
      savedVisualizationsAPI(project).update(input),
    onSuccess: () => client.invalidateQueries({ queryKey }),
  });
}
