package subscriptions

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCredentialImportAndOwnerBinding(t *testing.T) {
	now := time.Now()
	claims := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d,"https://api.openai.com/auth":{"chatgpt_account_id":"account"}}`, now.Add(time.Hour).Unix())))
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"header.` + claims + `.signature","account_id":"account","refresh_token":"never-store-this"}}`)
	c, err := Parse(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := Seal("secret", "alice", c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, c.AccessToken) {
		t.Fatal("plaintext persisted")
	}
	got, err := Open("secret", "alice", sealed)
	if err != nil || got != c {
		t.Fatal("round trip failed")
	}
	if _, err := Open("secret", "bob", sealed); err == nil {
		t.Fatal("cross-user decrypt")
	}
	if _, err := Open("wrong", "alice", sealed); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err := Parse(raw, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired accepted")
	}
	for _, bad := range []string{strings.Replace(string(raw), "chatgpt", "apikey", 1), strings.Replace(string(raw), `"account_id":"account"`, `"account_id":"other"`, 1), `{"auth_mode":"chatgpt","OPENAI_API_KEY":"key"}`} {
		if _, err := Parse([]byte(bad), now); err == nil {
			t.Fatal("invalid accepted")
		}
	}
}

func TestClaudeLoginAndRecordBinding(t *testing.T) {
	now := time.Now()
	raw := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat-test","expiresAt":%d,"scopes":["user:inference"],"refreshToken":"never-store"}}`, now.Add(time.Hour).UnixMilli())
	c, err := ParseProvider("claude", []byte(raw), now)
	if err != nil || c.ExpiresAt != now.Add(time.Hour).Unix() {
		t.Fatal("Claude import", err)
	}
	sealed, err := Seal("secret", Binding("user_alice", "sub_one"), c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Open("secret", Binding("user_alice", "sub_two"), sealed); err == nil {
		t.Fatal("credential moved between records")
	}
	for _, bad := range []string{strings.Replace(raw, "user:inference", "user:profile", 1), strings.Replace(raw, "sk-ant-oat-test", "sk-ant-api-key", 1), strings.Replace(raw, "sk-ant-oat-test", "sk-ant-oat-test\\n", 1)} {
		if _, err = ParseProvider("claude", []byte(bad), now); err == nil {
			t.Fatal("invalid Claude credential accepted")
		}
	}
	if _, err = ParseProvider("claude", []byte(raw), now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired Claude credential accepted")
	}
	if _, err = ParseProvider("codex", []byte(raw), now); err == nil {
		t.Fatal("wrong provider accepted")
	}
	if _, err = ParseProvider("other", []byte(raw), now); err == nil {
		t.Fatal("unknown provider accepted")
	}
}
