package egress

import (
	"net/http"
	"strings"
	"testing"

	"os"
	"os/exec"
)

func TestEnvironmentBypassesOnlyConfiguredPlatformHosts(t *testing.T) {
	if os.Getenv("WORKBENCH_TEST_PROXY_ENV") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestEnvironmentBypassesOnlyConfiguredPlatformHosts$")
		cmd.Env = append(os.Environ(), "WORKBENCH_TEST_PROXY_ENV=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("proxy environment check: %v\n%s", err, out)
		}
		return
	}

	env := Environment([]string{"HOME=/workspace"}, "http://10.43.180.206:8080", "https://broker.internal:8443", "http://[2001:db8::1]:8080", "http://*.example.com", "http://one,two", "https://user:secret@example.com")
	values := map[string]string{}
	for _, entry := range env {
		k, v, _ := strings.Cut(entry, "=")
		values[k] = v
	}
	if values["HOME"] != "/workspace" || values["NO_PROXY"] != values["no_proxy"] {
		t.Fatal("environment was not preserved")
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
	for _, tc := range []struct {
		url    string
		direct bool
	}{
		{"http://10.43.180.206:8080/codex/responses", true},
		{"https://broker.internal:8443/operations", true},
		{"http://[2001:db8::1]:8080", true},
		{"http://localhost:8080", true},
		{"https://example.com", false},
		{"http://10.43.180.207:8080", false},
		{"https://chatgpt.com", false},
	} {
		req, err := http.NewRequest("GET", tc.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := http.ProxyFromEnvironment(req)
		if err != nil {
			t.Fatal(err)
		}
		if (got == nil) != tc.direct {
			t.Errorf("%s: proxy %v, want direct %v", tc.url, got, tc.direct)
		}
	}
}
