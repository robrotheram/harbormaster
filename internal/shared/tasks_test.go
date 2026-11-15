package shared

import "testing"

func TestSanitizedTaskError(t *testing.T) {
	for _, code := range []string{"", "PROVIDER_NOT_CONFIGURED", "PROVIDER_LOGIN_REQUIRED", "CREDENTIAL_UNAVAILABLE", "AGENT_EXECUTABLE_UNAVAILABLE", "NATIVE_AGENT_FAILED", "RESULT_UNAVAILABLE", "INVALID_AGENT_RESPONSE", "STORAGE_UNAVAILABLE"} {
		if got := SanitizedTaskError(code); got != code {
			t.Errorf("SanitizedTaskError(%q) = %q", code, got)
		}
	}
	for _, code := range []string{"secret provider response", "PROVIDER_LOGIN_REQUIRED: token=secret", "unknown"} {
		if got := SanitizedTaskError(code); got != "EXECUTION_FAILED" {
			t.Errorf("untrusted error exposed: %q", got)
		}
	}
}
