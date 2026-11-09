package shared

import "time"

// HermesSetup contains project settings only. Provider/bot credentials and
// conversational content must never enter the platform metadata store.
type HermesSetup struct {
	ProjectID           string        `json:"project_id"`
	Provider            string        `json:"provider"`
	Model               string        `json:"model"`
	Mode                string        `json:"mode"`
	ConnectionProfileID string        `json:"connection_profile_id"`
	Matrix              *HermesMatrix `json:"matrix"`
	Revision            int           `json:"revision"`
	UpdatedAt           *time.Time    `json:"updated_at"`
}
type HermesMatrix struct {
	Homeserver string   `json:"homeserver"`
	BotUserID  string   `json:"bot_user_id"`
	RoomIDs    []string `json:"room_ids"`
	SenderIDs  []string `json:"sender_ids"`
}
