import { request } from "@/shared/http";

export type SetupOperation =
  | { operation: "tool-set"; name: string; version: string }
  | { operation: "tool-remove" | "skill-remove"; name: string }
  | { operation: "tools-restore" | "skills-restore" }
  | { operation: "skill-discover"; source: string }
  | { operation: "skill-add"; source: string; commit: string; name: string }
  | { operation: "apply" | "discard"; plan_id: string };

export type SetupJob = {
  id: string;
  state:
    "preparing" | "review" | "applied" | "discarded" | "discovered" | "failed";
  code?: string;
  request?: SetupOperation;
  review?: { changes: string[]; preview?: string };
  discovery?: { source: string; commit: string; names: string[] };
};

export type SetupInventory = {
  branch?: string;
  tool_config: string;
  tool_lock: string;
  tools_need_sync: boolean;
  tool_issue?: string;
  skill_lock: boolean;
  skill_issue?: string;
  skills_target?: string;
  tools: { name: string; requested: string; installed?: string }[];
  skills: {
    name: string;
    source?: string;
    commit?: string;
    state: "installed" | "missing" | "local";
  }[];
  catalog: string[];
  job?: SetupJob;
};

export function workerSetupPath(id: string) {
  return `/workspaces/${encodeURIComponent(id)}/setup`;
}

export function readWorkerSetup(id: string, signal?: AbortSignal) {
  return request<SetupInventory>(workerSetupPath(id), undefined, signal);
}

export function changeWorkerSetup(id: string, change: SetupOperation) {
  return request<SetupJob>(workerSetupPath(id), change);
}
