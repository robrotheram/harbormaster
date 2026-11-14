import { APIError } from "@/shared/http";
interface Snapshot {
  generated_at: string;
  services?: {
    items: {
      name: string;
      status: string;
      reason_code?: string;
      observed_at?: string;
    }[];
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
    const timeout = AbortSignal.timeout(15000);
    let r: Response;
    try {
      r = await fetch("/dashboard-api/v1/snapshot", {
        signal: AbortSignal.any([signal, timeout]),
        cache: "no-store",
      });
    } catch (error) {
      if (signal.aborted) throw error;
      throw new APIError(
        timeout.aborted ? "REQUEST_TIMEOUT" : "NETWORK_UNAVAILABLE",
        0,
      );
    }
    if (!r.ok) throw new APIError("UNAVAILABLE", r.status);
    try {
      return (await r.json()) as Snapshot;
    } catch {
      throw new APIError("UNAVAILABLE", r.status);
    }
  },
  refetchInterval: 5000,
});
