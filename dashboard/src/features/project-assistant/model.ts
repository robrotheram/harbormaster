import { requireProject } from "@/shared/http";
export interface HermesConfig {
  project_id: string;
  provider: "openai-codex";
  model: string;
  mode: "workspace" | "matrix" | "both";
  connection_profile_id: string;
  matrix: null | {
    homeserver: string;
    bot_user_id: string;
    room_ids: string[];
    sender_ids: string[];
  };
  revision: number;
  updated_at: string | null;
}
export interface HermesSnapshot {
  config: HermesConfig;
  saved: boolean;
  runtime_available: boolean;
  runtime_status: string;
  matrix_available: boolean;
  matrix_credential_configured?: boolean;
  matrix_state?: string;
  workspace_id?: string;
  workspace_ready?: boolean;
}
export function checkedHermes(value: HermesSnapshot, project: string) {
  requireProject([value.config], project);
  return value;
}
export function hermesRuntimeNotice(
  value: Pick<HermesSnapshot, "runtime_available" | "runtime_status">,
) {
  if (value.runtime_available) return null;
  if (value.runtime_status === "not_installed")
    return {
      label: "Runtime does not offer Hermes",
      title: "Hermes runtime unavailable",
      body: "The connected runtime does not offer Hermes. Check the runtime connection, then retry the status check. Your saved settings are kept.",
    };
  return {
    label: "Status check unavailable",
    title: "Could not check Hermes runtime",
    body: "Workbench could not reach the runtime to check Hermes. Retry the status check; your saved settings are kept.",
  };
}
