package shared

import (
	"strings"
	"testing"
)

func TestWorkSelection(t *testing.T) {
	for _, v := range []struct {
		provider, model string
		valid           bool
	}{
		{"", "", true}, {"codex", "", true}, {"claude", "model-version", true},
		{"codex", "provider/model", true}, {"", "model", false},
		{"unknown", "", false}, {"claude", " model", false},
		{"codex", "model\nargument", false}, {"codex", strings.Repeat("x", 201), false},
	} {
		if got := ValidWorkSelection(v.provider, v.model); got != v.valid {
			t.Errorf("%q / %q: %v", v.provider, v.model, got)
		}
	}
}
