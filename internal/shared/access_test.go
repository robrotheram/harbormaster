package shared

import (
	"strings"
	"testing"
	"time"
)

func TestProjectAccessCannotChangeProjectOrOutliveExpiry(t *testing.T) {
	now := time.Now()
	token, err := IssueProjectAccess("trusted-key", "project_a", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if p, err := VerifyProjectAccess("trusted-key", token, now); err != nil || p != "project_a" {
		t.Fatal("project authority lost")
	}
	for _, candidate := range []string{token + "x", strings.Replace(token, "wpa1.", "other.", 1), "wpa1.e30.invalid"} {
		if _, err = VerifyProjectAccess("trusted-key", candidate, now); err == nil {
			t.Fatal("forgery accepted")
		}
	}
	if _, err = VerifyProjectAccess("other-key", token, now); err == nil {
		t.Fatal("other authority accepted")
	}
	if _, err = VerifyProjectAccess("trusted-key", token, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired access accepted")
	}
}
