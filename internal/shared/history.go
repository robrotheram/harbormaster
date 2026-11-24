package shared

import "time"

// HistoryWait bounds a worker history request that waits for new records.
const HistoryWait = 5 * time.Second

// HistoryRecord contains private transcript data from the worker recorder.
type HistoryRecord struct {
	// Purpose is assigned by the control plane from operator metadata only.
	Purpose       string    `json:"purpose,omitempty"`
	CapturedAt    time.Time `json:"captured_at"`
	CaptureScope  string    `json:"capture_scope"`
	Backfilled    bool      `json:"backfilled,omitempty"`
	NativeRunID   string    `json:"native_run_id,omitempty"`
	RequesterID   string    `json:"requester_id,omitempty"`
	MessageID     string    `json:"message_id,omitempty"`
	Seq           int64     `json:"seq"`
	ID            string    `json:"id"`
	At            time.Time `json:"at"`
	EpisodeID     string    `json:"episode_id"`
	SessionID     string    `json:"session_id,omitempty"`
	Source        string    `json:"source"`
	Kind          string    `json:"kind"`
	Role          string    `json:"role,omitempty"`
	Content       string    `json:"content,omitempty"`
	ToolServer    string    `json:"tool_server,omitempty"`
	Tool          string    `json:"tool,omitempty"`
	ToolCallID    string    `json:"tool_call_id,omitempty"`
	Provider      string    `json:"provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	EngineVersion string    `json:"engine_version,omitempty"`
	Channel       string    `json:"channel,omitempty"`
	RoomID        string    `json:"room_id,omitempty"`
	SenderID      string    `json:"sender_id,omitempty"`
	Status        string    `json:"status,omitempty"`
	InputTokens   int64     `json:"input_tokens,omitempty"`
	OutputTokens  int64     `json:"output_tokens,omitempty"`
	UsageScope    string    `json:"usage_scope,omitempty"`
	ElapsedMS     int64     `json:"elapsed_ms,omitempty"`
	Synthetic     bool      `json:"synthetic,omitempty"`
	Redacted      bool      `json:"redacted,omitempty"`
	Truncated     bool      `json:"truncated,omitempty"`
	SchemaVersion int       `json:"schema_version"`
}
type HistoryPage struct {
	Records        []HistoryRecord `json:"records"`
	HighWatermark  int64           `json:"high_watermark"`
	NextAfter      int64           `json:"next_after"`
	NextBefore     int64           `json:"next_before"`
	BytesUsed      int64           `json:"bytes_used"`
	CaptureLimited bool            `json:"capture_limited"`
	SchemaVersion  int             `json:"schema_version"`
}
