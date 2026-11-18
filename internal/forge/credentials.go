package forge

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"

	"net/url"
	"strings"
)

// SealToken uses a purpose-separated key derived from the persistent control
// secret, which is kept outside the database. The account destination is bound
// as authenticated data so a stored credential cannot be moved to another host.
func tokenCipher(secret string) (cipher.AEAD, error) {
	if secret == "" {
		return nil, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "FORGE_CREDENTIAL_UNAVAILABLE"}
	}
	key := sha256.Sum256([]byte("workbench/forge-token/v1\x00" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func binding(h Host) []byte { return []byte(h.Name + "\x00" + h.Provider + "\x00" + h.APIURL) }
func SealToken(h Host, secret string) (string, error) {
	a, err := tokenCipher(secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(h.Token), binding(h))), nil
}
func OpenToken(h Host, secret, sealed string) (string, error) {
	a, err := tokenCipher(secret)
	if err != nil {
		return "", err
	}
	b, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(b) < a.NonceSize() {
		return "", shared.CodedError{Status: http.StatusServiceUnavailable, Code: "FORGE_CREDENTIAL_UNAVAILABLE"}
	}
	out, err := a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], binding(h))
	if err != nil {
		return "", shared.CodedError{Status: http.StatusServiceUnavailable, Code: "FORGE_CREDENTIAL_UNAVAILABLE"}
	}
	return string(out), nil
}

// NormalizeHost accepts GitLab's installation URL or its full API base URL.
func NormalizeHost(h Host) (Host, error) {
	h.Name = strings.TrimSpace(h.Name)
	h.APIURL = strings.TrimRight(strings.TrimSpace(h.APIURL), "/")
	h.Token = strings.TrimSpace(h.Token)
	if h.Provider == "github" && h.APIURL == "" {
		h.APIURL = "https://api.github.com"
	}
	if h.Provider == "gitlab" && h.APIURL == "" {
		h.APIURL = "https://gitlab.com"
	}
	u, err := url.Parse(h.APIURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" || strings.Contains(u.Path, "..") || strings.ContainsAny(h.APIURL, "\\\r\n\x00") || len(h.APIURL) > 2048 || len(h.Name) > 80 || h.Name == "" || strings.ContainsAny(h.Name, "\r\n\x00") || h.Token == "" || len(h.Token) > 8192 || strings.ContainsAny(h.Token, "\r\n\x00") || (h.Provider != "github" && h.Provider != "gitlab") {
		return Host{}, shared.CodedError{Status: http.StatusBadRequest, Code: "FORGE_ACCOUNT_INVALID"}
	}
	if h.Provider == "github" && h.APIURL != "https://api.github.com" {
		return Host{}, shared.CodedError{Status: http.StatusBadRequest, Code: "FORGE_ACCOUNT_INVALID"}
	}
	if h.Provider == "gitlab" && !strings.HasSuffix(h.APIURL, "/api/v4") {
		h.APIURL += "/api/v4"
	}
	return h, nil
}
func (c Client) Verify(ctx context.Context) error {
	var user struct {
		ID int64 `json:"id"`
	}
	if err := c.call(ctx, "GET", "/user", nil, &user); err != nil {
		return err
	}
	if user.ID <= 0 {
		return shared.CodedError{Status: http.StatusBadGateway, Code: "FORGE_RESPONSE_INVALID"}
	}
	return nil
}
