package shared

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"time"
)

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)
var safeKey = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127}$`)

func ValidID(s string) bool  { return safeID.MatchString(s) }
func ValidKey(s string) bool { return safeKey.MatchString(s) }
func NewID(prefix string) string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return prefix + hex.EncodeToString(b)
}

type Workspace struct {
	ConnectionProfileID       string            `json:"connection_profile_id,omitempty"`
	ConnectionProfileRevision int               `json:"connection_profile_revision,omitempty"`
	Connections               *ConnectionPolicy `json:"connections,omitempty"`
	ConnectionMode            string            `json:"connection_mode,omitempty"`
	ConnectionProxyVersion    int               `json:"connection_proxy_version,omitempty"`
	GitProxyReady             bool              `json:"git_proxy_ready"`
	IdleTimeoutMinutes        *int              `json:"idle_timeout_minutes,omitempty"`
	GitHubAccess              bool              `json:"github_access"`
	LastOpenedAt              *time.Time        `json:"last_opened_at,omitempty"`
	ConsoleEnabled            bool              `json:"console_enabled,omitempty"`
	HomeID                    string            `json:"home_id,omitempty"`
	AgentKind                 string            `json:"agent_kind,omitempty"`
	TemplateID                string            `json:"template_id,omitempty"`
	Name                      string            `json:"name,omitempty"`
	// ProfileID is read only as a legacy native-console marker; new workspaces never set it.
	ProfileID        string     `json:"profile_id,omitempty"`
	ParentID         string     `json:"parent_id,omitempty"`
	Branch           string     `json:"branch,omitempty"`
	ID               string     `json:"id"`
	ProjectID        string     `json:"project_id"`
	State            string     `json:"state"`
	Ready            bool       `json:"ready"`
	ActiveRuns       int        `json:"active_runs"`
	BackgroundJobs   int        `json:"background_jobs"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	IdleStopAt       *time.Time `json:"idle_stop_at"`
	RuntimeType      string     `json:"runtime_type"`
	IsolationStatus  string     `json:"isolation_status"`
	NetworkStatus    string     `json:"network_status"`
	CPULimit         float64    `json:"cpu_limit"`
	MemoryLimitBytes int64      `json:"memory_limit_bytes"`
	CPUCores         *float64   `json:"cpu_cores"`
	MemoryBytes      *int64     `json:"memory_bytes"`
	ObservedAt       *time.Time `json:"observed_at"`
}
type Task struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ErrorCode   string    `json:"error_code,omitempty"`
}

func Active(state string) bool {
	return state == "queued" || state == "running" || state == "cancel_requested"
}

type Activity struct {
	ActiveRuns     int       `json:"active_runs"`
	BackgroundJobs int       `json:"background_jobs"`
	Known          bool      `json:"known"`
	LastActivity   time.Time `json:"last_activity"`
}
type Observation struct {
	ConnectionMode         string   `json:"connection_mode,omitempty"`
	ConnectionProxyVersion int      `json:"connection_proxy_version,omitempty"`
	GitProxyReady          bool     `json:"git_proxy_ready"`
	Running                bool     `json:"running"`
	Ready                  bool     `json:"ready"`
	RuntimeType            string   `json:"runtime_type"`
	IsolationStatus        string   `json:"isolation_status"`
	NetworkStatus          string   `json:"network_status"`
	MemoryBytes            *int64   `json:"memory_bytes"`
	CPUCores               *float64 `json:"cpu_cores"`
	Activity               Activity `json:"activity"`
}
