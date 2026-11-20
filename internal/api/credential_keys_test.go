package api

import (
	"strings"
	"testing"

	"agent-workbench/internal/subscriptions"
)

func TestCredentialKeyringReadsOldAndLegacyRecords(t *testing.T) {
	s := &Server{cfg: Config{Token: strings.Repeat("x", 32)}}
	credential := subscriptions.Credential{AccessToken: "test-token"}
	legacy, err := s.sealSubscription("owner", "default", credential)
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.CredentialKeys = CredentialKeys{
		Active: "first",
		Keys:   map[string]string{"first": strings.Repeat("a", 32), "second": strings.Repeat("b", 32)},
	}
	first, err := s.sealSubscription("owner", "default", credential)
	if err != nil || !strings.HasPrefix(first, "first:") {
		t.Fatal("credential missing its key ID")
	}
	s.cfg.CredentialKeys.Active = "second"
	for _, record := range []string{legacy, first} {
		got, err := s.openSubscription("owner", "default", record)
		if err != nil || got.AccessToken != credential.AccessToken {
			t.Fatalf("old credential could not be opened: %v", err)
		}
	}
	if _, err := s.openSubscription("other", "default", first); err == nil {
		t.Fatal("keyed record lost its owner binding")
	}
}
