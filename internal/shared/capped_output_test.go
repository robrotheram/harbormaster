package shared

import (
	"io"
	"testing"
)

func TestCappedOutput(t *testing.T) {
	out := NewCappedOutput(4)
	write := func(chunk string) {
		t.Helper()
		if n, err := io.WriteString(out, chunk); n != len(chunk) || err != nil {
			t.Fatalf("write %q returned (%d, %v)", chunk, n, err)
		}
	}

	write("abc")
	write("d")
	write("")
	if string(out.Data) != "abcd" || out.Overflow {
		t.Fatalf("at cap: got %q, overflow %t", out.Data, out.Overflow)
	}
	write("ef")
	write("g")
	if string(out.Data) != "abcd" || !out.Overflow {
		t.Fatalf("past cap: got %q, overflow %t", out.Data, out.Overflow)
	}

	out = NewCappedOutput(4)
	write("abcé")
	if string(out.Data) != "abc\xc3" || !out.Overflow {
		t.Fatalf("split byte sequence: got %q, overflow %t", out.Data, out.Overflow)
	}
}
