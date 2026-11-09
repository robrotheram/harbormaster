package shared

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrAccess = errors.New("invalid project access")

type projectAccess struct {
	ProjectID string `json:"project_id"`
	Audience  string `json:"aud"`
	Expires   int64  `json:"exp"`
	Nonce     string `json:"nonce"`
}

// Project access is an application credential, distinct from provider logins.
// The trusted ingress and API share its signing authority; browsers do not.
func IssueProjectAccess(secret, projectID string, expiresAt time.Time) (string, error) {
	if secret == "" || !ValidID(projectID) || expiresAt.IsZero() {
		return "", ErrAccess
	}
	body, _ := json.Marshal(projectAccess{ProjectID: projectID, Audience: "workbench-project", Expires: expiresAt.Unix(), Nonce: NewID("access_")})
	payload := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("wpa1." + payload))
	return "wpa1." + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func VerifyProjectAccess(secret, token string, now time.Time) (string, error) {
	if secret == "" || len(token) > 4096 {
		return "", ErrAccess
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "wpa1" {
		return "", ErrAccess
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", ErrAccess
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return "", ErrAccess
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrAccess
	}
	var access projectAccess
	if json.Unmarshal(body, &access) != nil || access.Audience != "workbench-project" || !ValidID(access.ProjectID) || access.Expires <= now.Unix() || access.Nonce == "" {
		return "", ErrAccess
	}
	return access.ProjectID, nil
}
