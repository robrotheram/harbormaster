import { useQuery } from "@tanstack/react-query";
import { request } from "@/shared/http";
import type { GlobalConnectionProfile, ConnectionPolicy } from "@/shared/types";
export const connectionProfilesKey = ["global", "connection-profiles"] as const;
export function useConnectionProfiles() {
  return useQuery({
    queryKey: connectionProfilesKey,
    queryFn: ({ signal }) =>
      request<{ items: GlobalConnectionProfile[] }>(
        "/connection-profiles",
        undefined,
        signal,
      ).then((r) => r.items),
    refetchInterval: 15000,
  });
}

export const connectionProfilesApi = {
  save: (
    profile: GlobalConnectionProfile | null,
    payload: { name: string; description: string; policy: ConnectionPolicy },
  ) =>
    request(
      `/connection-profiles${profile ? `/${encodeURIComponent(profile.id)}` : ""}`,
      { ...payload, revision: profile?.revision ?? 0 },
    ),
};
