// Package subscriptions handles user-owned ChatGPT access-token snapshots.
// Refresh tokens and API keys are deliberately never persisted or distributed.
package subscriptions

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalid = errors.New("SUBSCRIPTION_INVALID")

type Credential struct {
	AccessToken string `json:"access_token"`
	AccountID   string `json:"account_id"`
	ExpiresAt   int64  `json:"expires_at"`
}

func Parse(data []byte, now time.Time) (Credential, error) {
	var file struct {
		AuthMode string  `json:"auth_mode"`
		APIKey   *string `json:"OPENAI_API_KEY"`
		Tokens   struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if len(data) > 64<<10 || json.Unmarshal(data, &file) != nil || file.AuthMode != "chatgpt" || (file.APIKey != nil && *file.APIKey != "") {
		return Credential{}, ErrInvalid
	}
	c := Credential{AccessToken: file.Tokens.AccessToken, AccountID: file.Tokens.AccountID}
	parts := strings.Split(c.AccessToken, ".")
	if len(parts) != 3 || len(c.AccountID) == 0 || len(c.AccountID) > 256 || strings.ContainsAny(c.AccessToken+c.AccountID, " \t\r\n\x00") {
		return Credential{}, ErrInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Credential{}, ErrInvalid
	}
	// Claims are used only for expiry and consistency, never to establish the
	// Workbench user's identity. The provider authenticates this bearer at use.
	var claims struct {
		Exp  int64 `json:"exp"`
		Auth struct {
			Account string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if json.Unmarshal(body, &claims) != nil || claims.Auth.Account != c.AccountID || claims.Exp <= now.Add(time.Minute).Unix() {
		return Credential{}, ErrInvalid
	}
	c.ExpiresAt = claims.Exp
	return c, nil
}
func crypt(secret string) (cipher.AEAD, error) {
	if secret == "" {
		return nil, ErrInvalid
	}
	key := sha256.Sum256([]byte("workbench/user-subscription/v1\x00" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func Seal(secret, owner string, c Credential) (string, error) {
	a, err := crypt(secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(a.Seal(nonce, nonce, b, []byte(owner))), nil
}
func Open(secret, owner, sealed string) (Credential, error) {
	a, err := crypt(secret)
	if err != nil {
		return Credential{}, err
	}
	b, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(b) < a.NonceSize() {
		return Credential{}, ErrInvalid
	}
	b, err = a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], []byte(owner))
	if err != nil {
		return Credential{}, ErrInvalid
	}
	var c Credential
	err = json.Unmarshal(b, &c)
	return c, err
}

// Binding preserves migrated credentials and binds new imports to their record.
func Binding(owner, id string) string {
	if id == "default" {
		return owner
	}
	return owner + "\x00" + id
}
func ValidProvider(provider string) bool { return provider == "codex" || provider == "claude" }
func ParseProvider(provider string, data []byte, now time.Time) (Credential, error) {
	if provider == "codex" {
		return Parse(data, now)
	}
	if provider != "claude" || len(data) > 64<<10 {
		return Credential{}, ErrInvalid
	}
	var file struct {
		OAuth struct {
			Token  string   `json:"accessToken"`
			Expiry int64    `json:"expiresAt"`
			Scopes []string `json:"scopes"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(data, &file) != nil {
		return Credential{}, ErrInvalid
	}
	c := Credential{AccessToken: file.OAuth.Token, ExpiresAt: file.OAuth.Expiry / 1000}
	if !strings.HasPrefix(c.AccessToken, "sk-ant-oat") || strings.ContainsAny(c.AccessToken, " \t\r\n\x00") || len(c.AccessToken) > 16384 || c.ExpiresAt <= now.Add(time.Minute).Unix() {
		return Credential{}, ErrInvalid
	}
	inference := false
	for _, scope := range file.OAuth.Scopes {
		if scope == "user:inference" {
			inference = true
		}
	}
	if !inference {
		return Credential{}, ErrInvalid
	}
	return c, nil
}
