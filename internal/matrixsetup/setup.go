// Package matrixsetup implements operator-driven onboarding, outside agent VMs.
package matrixsetup

import (
	"agent-workbench/internal/gateway"
	"agent-workbench/internal/shared"
	"bytes"
	"context"
	"encoding/json"

	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct{ HTTP *http.Client }

func New() *Client {
	t, _ := (gateway.NetworkPolicy{AllowedCIDRs: []string{"0.0.0.0/0", "::/0"}}).Transport(nil)
	return &Client{HTTP: &http.Client{Transport: t, Timeout: 10 * time.Second, CheckRedirect: gateway.NoRedirect}}
}
func origin(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_HOMESERVER_INVALID"}
	}
	return strings.TrimRight(raw, "/"), nil
}
func (c *Client) request(ctx context.Context, method, endpoint, token string, body any, out any) (int, error) {
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	r, e := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
	if e != nil {
		return 0, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "MATRIX_UNAVAILABLE"}
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", "application/json")
	res, e := c.HTTP.Do(r)
	if e != nil {
		return 0, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "MATRIX_UNAVAILABLE"}
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return res.StatusCode, nil
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if e != nil || len(b) > 1<<20 || json.Unmarshal(b, out) != nil {
		return res.StatusCode, shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_HOMESERVER_INVALID"}
	}
	return res.StatusCode, nil
}

// Discovery never receives a token and never follows a redirect.
func (c *Client) Discover(ctx context.Context, raw string) (string, error) {
	base, e := origin(raw)
	if e != nil {
		return "", e
	}
	var doc struct {
		Homeserver struct {
			URL string `json:"base_url"`
		} `json:"m.homeserver"`
	}
	status, e := c.request(ctx, "GET", base+"/.well-known/matrix/client", "", nil, &doc)
	if e == nil && status == 200 && doc.Homeserver.URL != "" {
		base, e = origin(doc.Homeserver.URL)
		if e != nil {
			return "", e
		}
	}
	var versions struct {
		Versions []string `json:"versions"`
	}
	status, e = c.request(ctx, "GET", base+"/_matrix/client/versions", "", nil, &versions)
	if e != nil || status != 200 || len(versions.Versions) == 0 {
		return "", shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_HOMESERVER_INVALID"}
	}
	return base, nil
}

// Prepare accepts invitations only on explicit operator request, only after
// checking identity, and only for exact configured room IDs. The guest broker
// continues to forbid all joins/invites.
func (c *Client) Prepare(ctx context.Context, config shared.HermesMatrix, token string, accept bool) error {
	base, e := origin(config.Homeserver)
	if e != nil {
		return e
	}
	if token == "" || len(token) > 8192 || strings.ContainsAny(token, " \r\n\t") {
		return shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_TOKEN_INVALID"}
	}
	// Verify the configured origin without credentials before submitting a token.
	var versions struct {
		Versions []string `json:"versions"`
	}
	status, e := c.request(ctx, "GET", base+"/_matrix/client/versions", "", nil, &versions)
	if e != nil || status != 200 || len(versions.Versions) == 0 {
		return shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_HOMESERVER_INVALID"}
	}
	var who struct {
		ID string `json:"user_id"`
	}
	status, e = c.request(ctx, "GET", base+"/_matrix/client/v3/account/whoami", token, nil, &who)
	if e != nil {
		return e
	}
	if status == 401 || status == 403 {
		return shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_TOKEN_INVALID"}
	}
	if status != 200 {
		return shared.CodedError{Status: http.StatusServiceUnavailable, Code: "MATRIX_UNAVAILABLE"}
	}
	if who.ID != config.BotUserID {
		return shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_BOT_ID_MISMATCH"}
	}
	var joined struct {
		Rooms []string `json:"joined_rooms"`
	}
	status, e = c.request(ctx, "GET", base+"/_matrix/client/v3/joined_rooms", token, nil, &joined)
	if e != nil {
		return e
	}
	if status != 200 {
		return shared.CodedError{Status: http.StatusServiceUnavailable, Code: "MATRIX_UNAVAILABLE"}
	}
	allowed, member := map[string]bool{}, map[string]bool{}
	for _, id := range config.RoomIDs {
		allowed[id] = true
	}
	for _, id := range joined.Rooms {
		if !allowed[id] {
			return shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_BOT_OUTSIDE_ROOMS"}
		}
		member[id] = true
	}
	missing := []string{}
	for _, id := range config.RoomIDs {
		if !member[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		// Request only membership state for the configured rooms, without history.
		filter, _ := json.Marshal(map[string]any{"presence": map[string]any{"types": []string{}}, "account_data": map[string]any{"types": []string{}}, "room": map[string]any{"rooms": config.RoomIDs, "timeline": map[string]int{"limit": 0}, "state": map[string]any{"types": []string{"m.room.member"}}, "ephemeral": map[string]any{"types": []string{}}, "account_data": map[string]any{"types": []string{}}}})
		var sync struct {
			Rooms struct {
				Invite map[string]struct {
					State struct {
						Events []struct {
							Type    string `json:"type"`
							Key     string `json:"state_key"`
							Content struct {
								Membership string `json:"membership"`
							} `json:"content"`
						} `json:"events"`
					} `json:"invite_state"`
				} `json:"invite"`
			} `json:"rooms"`
		}
		status, e = c.request(ctx, "GET", base+"/_matrix/client/v3/sync?timeout=0&filter="+url.QueryEscape(string(filter)), token, nil, &sync)
		if e != nil {
			return e
		}
		if status != 200 {
			return shared.CodedError{Status: http.StatusServiceUnavailable, Code: "MATRIX_UNAVAILABLE"}
		}
		for _, id := range missing {
			invited := false
			for _, event := range sync.Rooms.Invite[id].State.Events {
				if event.Type == "m.room.member" && event.Key == config.BotUserID && event.Content.Membership == "invite" {
					invited = true
				}
			}
			if !invited {
				return shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_INVITE_REQUIRED"}
			}
		}
		if !accept {
			return shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_ACCEPT_INVITATIONS_REQUIRED"}
		}
		for _, id := range missing {
			var result struct {
				ID string `json:"room_id"`
			}
			status, e = c.request(ctx, "POST", base+"/_matrix/client/v3/join/"+url.PathEscape(id), token, map[string]any{}, &result)
			if e != nil {
				return e
			}
			if status != 200 || result.ID != id {
				return shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_INVITE_REQUIRED"}
			}
		}
	}
	for _, id := range config.RoomIDs {
		var encryption struct {
			Algorithm string `json:"algorithm"`
		}
		status, e = c.request(ctx, "GET", base+"/_matrix/client/v3/rooms/"+url.PathEscape(id)+"/state/m.room.encryption", token, nil, &encryption)
		if e != nil {
			return e
		}
		if status != 200 || encryption.Algorithm != "m.megolm.v1.aes-sha2" {
			return shared.CodedError{Status: http.StatusBadRequest, Code: "MATRIX_ROOM_ENCRYPTION_REQUIRED"}
		}
	}
	return nil
}
