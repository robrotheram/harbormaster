export interface Project {
  deployment_namespace?: string;
  namespace_ready?: boolean;
  id: string;
  name: string;
  created_at: string;
}
export interface ConnectionPolicy {
  mode: "blocked" | "allowlist" | "open";
  destinations: string[];
}
export interface GlobalConnectionProfile {
  id: string;
  name: string;
  description: string;
  policy: ConnectionPolicy;
  revision: number;
  updated_at: string;
}
export interface Workspace {
  shared?: boolean;
  retained_preview?: {
    service_id: string;
    source_workspace_id: string;
    artifact_id: string;
    expires_at: number;
  };
  connection_profile_id?: string;
  connection_profile_revision?: number;
  connections?: ConnectionPolicy;
  connection_mode?: string;
  idle_timeout_minutes?: number;
  console_enabled?: boolean;
  id: string;
  project_id: string;
  name?: string;
  template_id?: string;
  agent_kind?: "t3" | "codex" | "hermes";
  home_id?: string;
  parent_id?: string;
  branch?: string;
  state: string;
  ready: boolean;
  active_runs: number;
  background_jobs: number;
  created_at: string;
  updated_at: string;
  idle_stop_at?: string | null;
  runtime_type: string;
  isolation_status: string;
  network_status: string;
  cpu_limit: number;
  memory_limit_bytes: number;
  cpu_cores?: number | null;
  memory_bytes?: number | null;
}
export interface Schedule {
  revision?: number;
  id: string;
  project_id: string;
  workspace_id: string;
  name: string;
  instruction_path: string;
  provider: string;
  enabled: boolean;
  run_at: string;
  interval_seconds: number;
  next_run_at?: string | null;
  last_run_at?: string | null;
  last_error_code?: string;
  created_at: string;
}
export interface Run {
  id: string;
  project_id: string;
  workspace_id: string;
  schedule_id?: string;
  task_id?: string;
  state: string;
  created_at: string;
  updated_at: string;
  error_code?: string;
}
export interface StartResult extends Workspace {
  console_ticket?: string;
}
export interface Pairing {
  pairing_url: string;
  expires_at: string;
}
export type Section =
  | "team"
  | "board"
  | "reviews"
  | "work"
  | "workspaces"
  | "settings"
  | "overview"
  | "help";
