interface Snapshot {
  generated_at: string;
  services?: {
    items: { name: string; status: string; reason_code?: string }[];
  };
  usage?: {
    running_workspaces?: number;
    max_running_workspaces?: number;
    cpu_allocated?: number;
    memory_allocated_bytes?: number;
  };
}
export const snapshotOptions = () => ({
  queryKey: ["platform", "snapshot"],
  queryFn: async ({ signal }: { signal: AbortSignal }) => {
    const r = await fetch("/dashboard-api/v1/snapshot", {
      signal,
      cache: "no-store",
    });
    if (!r.ok)
      throw Error("Runtime information is unavailable. Retry shortly.");
    return (await r.json()) as Snapshot;
  },
  refetchInterval: 5000,
});
