package orchestration

// Schema defines the orchestration tables in the platform metadata database.
const Schema = `CREATE TABLE IF NOT EXISTS schedules (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, create_key TEXT NOT NULL, metadata TEXT NOT NULL, UNIQUE(project_id,create_key));
CREATE TABLE IF NOT EXISTS orchestration_runs (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, schedule_id TEXT NOT NULL, state TEXT NOT NULL, created_at TEXT NOT NULL, metadata TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS orchestration_runs_state ON orchestration_runs(state,created_at);
CREATE TABLE IF NOT EXISTS work_items (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, create_key TEXT NOT NULL, metadata TEXT NOT NULL, UNIQUE(project_id,create_key));
CREATE TABLE IF NOT EXISTS work_create_requests (project_id TEXT NOT NULL,create_key TEXT NOT NULL,fingerprint TEXT NOT NULL,PRIMARY KEY(project_id,create_key));
CREATE TABLE IF NOT EXISTS work_attempts(id TEXT PRIMARY KEY, work_item_id TEXT NOT NULL, project_id TEXT NOT NULL, workspace_id TEXT NOT NULL, delivery_key TEXT NOT NULL, active_workspace TEXT UNIQUE, metadata TEXT NOT NULL, UNIQUE(work_item_id,delivery_key));`
