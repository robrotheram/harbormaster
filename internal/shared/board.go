package shared

// BoardCreate saves a standalone planned issue without dispatching an attempt.
type BoardCreate struct {
	ProjectID      string `json:"project_id"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Kind           string `json:"kind"`
	IdempotencyKey string `json:"idempotency_key"`
}
