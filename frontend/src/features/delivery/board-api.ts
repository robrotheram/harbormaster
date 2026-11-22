import { request } from "@/shared/http";
import type { WorkItem } from "@/features/work/api";
export const columns = [
  { id: "planned", label: "Planned" },
  { id: "running", label: "In progress" },
  { id: "review", label: "Review" },
  { id: "done", label: "Done" },
] as const;
export type BoardStage = (typeof columns)[number]["id"] | "archived";
export type BoardCard = {
  number?: number;
  owner_id?: string;
  owner?: string;
  priority?: string;
  dependencies?: string[];
  require_dependencies?: boolean;
  id: string;
  project_id: string;
  revision: number;
  stage: BoardStage;
  archived_from?: BoardStage;
  title?: string;
  description?: string;
  dirty: boolean;
  attention: boolean;
  work: WorkItem;
  source?: {
    connection_id: string;
    repository: string;
    number: number;
    url: string;
    title: string;
    state: string;
    synced_at: string;
    error?: string;
  };
};
export const boardKey = (project: string) => ["project", project, "board"];
export const boardOptions = (project: string) => ({
  queryKey: boardKey(project),
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    request<{ items: BoardCard[] }>(
      `/board?project_id=${encodeURIComponent(project)}`,
      undefined,
      signal,
    ),
  refetchInterval: 5000,
});
export const boardApi = {
  save: (
    card: BoardCard,
    stage: BoardStage,
    title?: string,
    description?: string,
    coordination?: {
      owner?: string;
      owner_id?: string;
      priority?: string;
      dependencies?: string[];
      require_dependencies?: boolean;
    },
  ) =>
    request<BoardCard>(`/board/${card.id}`, {
      project_id: card.project_id,
      revision: card.revision,
      stage,
      ...coordination,
      ...(title !== undefined ? { title } : {}),
      ...(description !== undefined ? { description } : {}),
    }),
  sync: (card: BoardCard, direction: "pull" | "push", discard_local = false) =>
    request<BoardCard>(`/board/${card.id}/sync`, {
      project_id: card.project_id,
      revision: card.revision,
      direction,
      discard_local,
    }),
};
