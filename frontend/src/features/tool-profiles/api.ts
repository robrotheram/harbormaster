import { request } from "@/shared/http";
import { keys } from "@/shared/query-keys";
import type { Profile } from "@/shared/types";
export const toolProfilesApi = {
  list: (signal?: AbortSignal) =>
    request<{ items: Profile[] }>("/profiles", undefined, signal).then(
      (x) => x.items,
    ),
  save: (
    profile: Profile | null,
    payload: Pick<Profile, "title" | "description" | "mcps">,
  ) =>
    request(
      `/tool-profiles${profile ? `/${encodeURIComponent(profile.id)}` : ""}`,
      { ...payload, revision: profile?.revision || 0 },
    ),
};
export const toolProfileOptions = () => ({
  queryKey: keys.profiles,
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    toolProfilesApi.list(signal),
  refetchInterval: 15000,
});
