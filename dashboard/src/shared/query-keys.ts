export const keys = {
  projects: ["projects"] as const,
  project: (project: string) => ["project", project] as const,
  workspaces: (project: string) => ["project", project, "workspaces"] as const,
  schedules: (project: string) => ["project", project, "schedules"] as const,
  runs: (project: string) => ["project", project, "runs"] as const,
};
