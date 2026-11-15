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
