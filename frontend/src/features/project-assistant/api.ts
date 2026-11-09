import { request } from "@/shared/http";
import type { Workspace } from "@/shared/types";
import { checkedHermes, type HermesConfig, type HermesSnapshot } from "./model";
export const assistantKey = (project: string) =>
  ["project", project, "hermes"] as const;
export const assistantApi = {
  read: (project: string, signal?: AbortSignal) =>
    request<HermesSnapshot>(
      `/projects/${encodeURIComponent(project)}/hermes`,
      undefined,
      signal,
    ).then((v) => checkedHermes(v, project)),
  save: async (project: string, config: HermesConfig) => {
    if (config.matrix) {
      const found = await request<{ homeserver: string }>(
        `/projects/${encodeURIComponent(project)}/hermes/discover`,
        { homeserver: config.matrix.homeserver },
      );
      config = {
        ...config,
        matrix: { ...config.matrix, homeserver: found.homeserver },
      };
    }
    return checkedHermes(
      await request<HermesSnapshot>(
        `/projects/${encodeURIComponent(project)}/hermes`,
        config,
      ),
      project,
    );
  },
  start: (project: string, idempotency_key: string) =>
    request<Workspace>(
      `/projects/${encodeURIComponent(project)}/hermes/assistant`,
      { idempotency_key },
    ),
  saveMatrixCredential: (
    project: string,
    access_token: string,
    revision: number,
    accept_invitations: boolean,
  ) =>
    request(
      `/projects/${encodeURIComponent(project)}/hermes/matrix-credential`,
      { access_token, revision, accept_invitations },
    ),
  matrixAction: (project: string, action: "start" | "stop") =>
    request(
      `/projects/${encodeURIComponent(project)}/hermes/matrix/${action}`,
      {},
    ),
};
export const assistantOptions = (project: string) => ({
  queryKey: assistantKey(project),
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    assistantApi.read(project, signal),
  refetchInterval: 10000,
  refetchIntervalInBackground: true,
});
