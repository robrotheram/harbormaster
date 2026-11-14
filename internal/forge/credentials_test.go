package forge

import (
	"strings"
	"testing"
)

func TestCredentialEncryptionBindsDestination(t *testing.T) {
	h := Host{Name: "Office", Provider: "gitlab", APIURL: "https://gitlab.example/api/v4", Token: "synthetic-private-token"}
	sealed, err := SealToken(h, "server-secret")
	if err != nil || strings.Contains(sealed, h.Token) {
		t.Fatal("credential not encrypted", err)
	}
	again, _ := SealToken(h, "server-secret")
	if sealed == again {
		t.Fatal("nonce reused")
	}
	token, err := OpenToken(h, "server-secret", sealed)
	if err != nil || token != h.Token {
		t.Fatal("round trip failed", err)
	}
	h.APIURL = "https://other.example/api/v4"
	if _, err = OpenToken(h, "server-secret", sealed); err == nil {
		t.Fatal("destination substitution allowed")
	}
	h.APIURL = "https://gitlab.example/api/v4"
	if _, err = OpenToken(h, "other-secret", sealed); err == nil {
		t.Fatal("wrong key accepted")
	}
}
func TestNormalizeAccountDestinations(t *testing.T) {
	for _, test := range []struct{ provider, url, want string }{
		{"github", "", "https://api.github.com"},
		{"gitlab", "", "https://gitlab.com/api/v4"},
		{"gitlab", "https://office.example/gitlab/", "https://office.example/gitlab/api/v4"},
		{"gitlab", "https://office.example/api/v4", "https://office.example/api/v4"},
	} {
		h, err := NormalizeHost(Host{Name: "Account", Provider: test.provider, APIURL: test.url, Token: "token"})
		if err != nil || h.APIURL != test.want {
			t.Fatal(h.APIURL, err)
		}
	}
	for _, address := range []string{"http://office.example", "https://user:secret@office.example", "https://office.example?token=x", "https://office.example/#fragment", "https://office.example/../other", "https://office.example/%2e%2e/other"} {
		if _, err := NormalizeHost(Host{Name: "Account", Provider: "gitlab", APIURL: address, Token: "token"}); err == nil {
			t.Fatal("unsafe URL accepted", address)
		}
	}
}
