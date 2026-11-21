import { request } from "@/shared/http";
export const capabilityOptions = (project: string) => ({
  queryKey: ["project", project, "capabilities"],
  queryFn: () =>
    request<{
      can_import: boolean;
      can_start: boolean;
      requires_agent_selection?: boolean;
      import_reason: string;
      start_reason: string;
    }>(`/projects/${encodeURIComponent(project)}/capabilities`),
  retry: false,
});
