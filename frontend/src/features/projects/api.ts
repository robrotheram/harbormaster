import { request } from "@/shared/http";
import type { Project } from "@/shared/types";
export const projectsApi = {
  list: (signal?: AbortSignal) =>
    request<{ items: Project[] }>("/projects", undefined, signal).then((x) =>
      x.items.map((project) =>
        project.id === "default" ? { ...project, name: "Workspaces" } : project,
      ),
    ),
  create: (name: string, idempotency_key: string) =>
    request<Project>("/projects", { name, idempotency_key }),
};
