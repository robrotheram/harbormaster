import { request } from "@/shared/http";
import type { Project } from "@/shared/types";
export const projectsApi = {
  list: (signal?: AbortSignal) =>
    request<{ items: Project[] }>("/projects", undefined, signal).then((x) =>
      x.items.map((project) =>
        project.id === "default" && project.name === "Legacy workspaces"
          ? { ...project, name: "Workspaces" }
          : project,
      ),
    ),
  rename: (id: string, name: string) =>
    request<Project>(`/projects/${encodeURIComponent(id)}/rename`, { name }),
  create: (name: string, idempotency_key: string, sample = false) =>
    request<Project>("/projects", {
      name,
      idempotency_key,
      ...(sample ? { sample: true } : {}),
    }),
};
