package shared

import "time"

type ConnectionProfile struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Policy      ConnectionPolicy `json:"policy"`
	Revision    int              `json:"revision"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

func DefaultConnectionProfiles() []ConnectionProfile {
	return []ConnectionProfile{
		{ID: "blocked", Name: "Blocked", Description: "Platform integrations only.", Policy: ConnectionPolicy{Mode: "blocked", Destinations: []string{}}, Revision: 1},
		{ID: "github", Name: "GitHub only", Description: "GitHub HTTPS Git through the proxy.", Policy: ConnectionPolicy{Mode: "allowlist", Destinations: []string{"github.com:443"}}, Revision: 1},
		{ID: "custom", Name: "Development allowlist", Description: "Edit these approved destinations for development work.", Policy: ConnectionPolicy{Mode: "allowlist", Destinations: []string{"github.com:443", "registry.npmjs.org:443"}}, Revision: 1},
		{ID: "open", Name: "Fully open", Description: "Unrestricted direct networking and DNS.", Policy: ConnectionPolicy{Mode: "open", Destinations: []string{}}, Revision: 1},
	}
}
