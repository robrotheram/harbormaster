package shared

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/mail"
	"strings"
	"time"
)

type HumanIdentity struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}
type humanAccess struct {
	HumanIdentity
	Audience string `json:"aud"`
	Expires  int64  `json:"exp"`
}

func ValidHuman(h HumanIdentity) bool {
	address, err := mail.ParseAddress(h.Email)
	return ValidID(h.ID) && err == nil && address.Address == h.Email && h.Email == strings.ToLower(h.Email) && len(h.Email) <= 254
}
func IssueHumanAccess(secret string, human HumanIdentity, expiry time.Time) (string, error) {
	if secret == "" || !ValidHuman(human) {
		return "", ErrAccess
	}
	body, _ := json.Marshal(humanAccess{human, "workbench-human", expiry.Unix()})
	payload := "wha1." + base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func VerifyHumanAccess(secret, token string, now time.Time) (HumanIdentity, error) {
	var claim humanAccess
	parts := strings.Split(token, ".")
	if secret == "" || len(token) > 4096 || len(parts) != 3 || parts[0] != "wha1" {
		return claim.HumanIdentity, ErrAccess
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claim.HumanIdentity, ErrAccess
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return claim.HumanIdentity, ErrAccess
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(body, &claim) != nil || claim.Audience != "workbench-human" || claim.Expires <= now.Unix() || !ValidHuman(claim.HumanIdentity) {
		return HumanIdentity{}, ErrAccess
	}
	return claim.HumanIdentity, nil
}
