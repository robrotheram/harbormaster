package matrixsetup

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(r *http.Request, status int, body any) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}, Request: r}
}
func TestDiscoveryNeverSendsCredentialsAndVerifiesDiscoveredServer(t *testing.T) {
	calls := []string{}
	c := &Client{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("credential sent during discovery")
		}
		calls = append(calls, r.URL.String())
		if r.URL.Host == "element.example.org" {
			return response(r, 200, map[string]any{"m.homeserver": map[string]string{"base_url": "https://matrix.example.org"}}), nil
		}
		if r.URL.Host != "matrix.example.org" || r.URL.Path != "/_matrix/client/versions" {
			t.Fatal("unexpected destination", r.URL)
		}
		return response(r, 200, map[string]any{"versions": []string{"v1.17"}}), nil
	})}}
	got, err := c.Discover(context.Background(), "https://element.example.org")
	if err != nil || got != "https://matrix.example.org" || len(calls) != 2 {
		t.Fatal(got, err, calls)
	}
}
func TestDiscoveryRejectsPrivateDestinationsAndInvalidURLs(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	for _, input := range []string{server.URL, "http://example.org", "https://user:pass@example.org", "https://example.org?token=secret"} {
		if _, err := New().Discover(context.Background(), input); err == nil {
			t.Fatal("unsafe discovery accepted", input)
		}
	}
	if calls != 0 {
		t.Fatal("private destination reached")
	}
}
func TestOnboardingOnlyAcceptsExplicitInvitationsForConfiguredRooms(t *testing.T) {
	for _, tc := range []struct {
		name                                          string
		accept, invite, wrong, foreign, plain, second bool
		want                                          string
		joins                                         int
	}{
		{name: "invite missing", accept: true, want: "MATRIX_INVITE_REQUIRED"},
		{name: "operator consent required", invite: true, want: "MATRIX_ACCEPT_INVITATIONS_REQUIRED"},
		{name: "accept configured invite", invite: true, accept: true, joins: 1},
		{name: "wrong identity", invite: true, accept: true, wrong: true, want: "MATRIX_BOT_ID_MISMATCH"},
		{name: "foreign joined room", invite: true, accept: true, foreign: true, want: "MATRIX_BOT_OUTSIDE_ROOMS"},
		{name: "unencrypted room", invite: true, accept: true, plain: true, want: "MATRIX_ROOM_ENCRYPTION_REQUIRED", joins: 1},
		{name: "all invites checked before joins", invite: true, accept: true, second: true, want: "MATRIX_INVITE_REQUIRED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := shared.HermesMatrix{Homeserver: "https://matrix.example.org", BotUserID: "@bot:example.org", RoomIDs: []string{"!approved:example.org"}}
			if tc.second {
				config.RoomIDs = append(config.RoomIDs, "!second:example.org")
			}
			joins := 0
			c := &Client{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/_matrix/client/versions" {
					if r.Header.Get("Authorization") != "" {
						t.Fatal("token sent before origin check")
					}
					return response(r, 200, map[string]any{"versions": []string{"v1.17"}}), nil
				}
				if r.Header.Get("Authorization") != "Bearer synthetic-token" {
					t.Fatal("missing credential")
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/whoami"):
					id := config.BotUserID
					if tc.wrong {
						id = "@other:example.org"
					}
					return response(r, 200, map[string]string{"user_id": id}), nil
				case strings.HasSuffix(r.URL.Path, "/joined_rooms"):
					rooms := []string{}
					if tc.foreign {
						rooms = append(rooms, "!foreign:example.org")
					}
					return response(r, 200, map[string]any{"joined_rooms": rooms}), nil
				case strings.HasSuffix(r.URL.Path, "/sync"):
					var filter struct {
						Room struct {
							Rooms    []string `json:"rooms"`
							Timeline struct {
								Limit int `json:"limit"`
							} `json:"timeline"`
						} `json:"room"`
					}
					if json.Unmarshal([]byte(r.URL.Query().Get("filter")), &filter) != nil || len(filter.Room.Rooms) != len(config.RoomIDs) || filter.Room.Timeline.Limit != 0 {
						t.Fatal("sync not restricted")
					}
					invites := map[string]any{}
					event := map[string]any{"invite_state": map[string]any{"events": []any{map[string]any{"type": "m.room.member", "state_key": config.BotUserID, "content": map[string]string{"membership": "invite"}}}}}
					invites["!foreign:example.org"] = event
					if tc.invite {
						invites[config.RoomIDs[0]] = event
					}
					return response(r, 200, map[string]any{"rooms": map[string]any{"invite": invites}}), nil
				case strings.Contains(r.URL.Path, "/join/"):
					if r.Method != "POST" || r.URL.Path != "/_matrix/client/v3/join/"+config.RoomIDs[0] {
						t.Fatal("joined unapproved room", r.URL)
					}
					joins++
					return response(r, 200, map[string]string{"room_id": config.RoomIDs[0]}), nil
				case strings.HasSuffix(r.URL.Path, "/state/m.room.encryption"):
					algo := "m.megolm.v1.aes-sha2"
					if tc.plain {
						algo = ""
					}
					return response(r, 200, map[string]string{"algorithm": algo}), nil
				default:
					t.Fatal("unexpected request", r.URL)
					return nil, nil
				}
			})}}
			err := c.Prepare(context.Background(), config, "synthetic-token", tc.accept)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tc.want || joins != tc.joins {
				t.Fatal("unexpected outcome", got, joins)
			}
		})
	}
}
