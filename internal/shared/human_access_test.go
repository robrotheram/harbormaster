package shared

import (
	"strings"
	"testing"
	"time"
)

func TestHumanIdentityCannotBeForgedOrReusedAfterExpiry(t *testing.T) {
	now := time.Now()
	h := HumanIdentity{ID: "user_alice", Email: "alice@example.com"}
	token, err := IssueHumanAccess("secret", h, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyHumanAccess("secret", token, now)
	if err != nil || got != h {
		t.Fatal(got, err)
	}
	for _, candidate := range []string{token + "x", strings.Replace(token, "wha1", "wpa1", 1), "garbage"} {
		if _, err := VerifyHumanAccess("secret", candidate, now); err == nil {
			t.Fatal("forged identity")
		}
	}
	if _, err := VerifyHumanAccess("other", token, now); err == nil {
		t.Fatal("wrong signer")
	}
	if _, err := VerifyHumanAccess("secret", token, now.Add(2*time.Minute)); err == nil {
		t.Fatal("expired identity")
	}
}
