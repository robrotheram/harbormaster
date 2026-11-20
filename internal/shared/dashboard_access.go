package shared

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// DashboardReadToken derives a credential scoped to the dashboard snapshot.
func DashboardReadToken(controlToken string) string {
	if controlToken == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(controlToken))
	mac.Write([]byte("harbormaster:dashboard-snapshot:v1"))
	return hex.EncodeToString(mac.Sum(nil))
}
