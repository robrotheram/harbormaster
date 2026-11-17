import {
  retryTransient,
  pollUnlessBlocked,
  refocusUnlessBlocked,
} from "@/shared/query-policy";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/shared/http";
import type { GlobalConnectionProfile, ConnectionPolicy } from "@/shared/types";
export const connectionProfilesKey = ["global", "connection-profiles"] as const;
export function useConnectionProfiles(enabled = true) {
  return useQuery({
    queryKey: connectionProfilesKey,
    enabled,
    retry: retryTransient,
    retryOnMount: false,
    refetchOnWindowFocus: refocusUnlessBlocked,
    refetchOnReconnect: refocusUnlessBlocked,
    refetchOnMount: refocusUnlessBlocked,
    queryFn: ({ signal }) =>
      request<{ items: GlobalConnectionProfile[] }>(
        "/connection-profiles",
        undefined,
        signal,
      ).then((r) => r.items),
    refetchInterval: pollUnlessBlocked(15000),
  });
}

export const connectionProfilesApi = {
  delete: (profile: GlobalConnectionProfile) =>
    request(`/connection-profiles/${encodeURIComponent(profile.id)}/delete`, {
      revision: profile.revision,
    }),
  save: (
    profile: GlobalConnectionProfile | null,
    payload: { name: string; description: string; policy: ConnectionPolicy },
  ) =>
    request(
      `/connection-profiles${profile ? `/${encodeURIComponent(profile.id)}` : ""}`,
      { ...payload, revision: profile?.revision ?? 0 },
    ),
};
