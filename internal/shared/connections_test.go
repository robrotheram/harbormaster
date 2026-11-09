package shared

import "testing"

func TestConnectionPolicyBoundsAndExactMatching(t *testing.T) {
	p := ConnectionPolicy{Mode: "allowlist", Destinations: []string{"github.com:443", "registry.npmjs.org:443", "[2001:db8::1]:8443"}}
	if !p.Valid() || !p.Allows("github.com:443") || p.Allows("github.com.evil.test:443") || p.Allows("github.com:80") {
		t.Fatal("allowlist matching failed")
	}
	for _, d := range []string{"https://github.com", "*.github.com:443", "github.com:0", "github.com:65536", "github.com:0443", "github.com:443/path", "github.com.:443", "GitHub.com:443", "-bad.test:443", "github.com:443\r\nHost:evil"} {
		if ValidDestination(d) {
			t.Fatal("accepted invalid target", d)
		}
	}
	if (ConnectionPolicy{Mode: "open", Destinations: []string{"x:1"}}).Valid() {
		t.Fatal("open mode accepted ambiguous rules")
	}
	if !(ConnectionPolicy{Mode: "open"}).Allows("example.com:443") || (ConnectionPolicy{Mode: "blocked"}).Allows("github.com:443") {
		t.Fatal("mode semantics broken")
	}
}
