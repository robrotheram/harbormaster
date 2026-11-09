package workspace

import "testing"

func TestKernelInfrastructureUsesFlagsNotNames(t *testing.T) {
	for _, tt := range []struct {
		stat string
		want bool
	}{
		{"2 (kthreadd) S 0 0 0 0 0 2097152", true},
		{"80 (bpfilter_umh) S 1 0 0 0 0 33554432", true},
		{"81 (kthreadd) S 1 0 0 0 0 4194304", false},
		{"82 (a ) strange name) S 1 0 0 0 0 4194304", false},
		{"malformed", false},
	} {
		if got := kernelInfrastructure(tt.stat); got != tt.want {
			t.Errorf("%q: got %v want %v", tt.stat, got, tt.want)
		}
	}
}

func TestKernelHelpersRequireRealKernelParent(t *testing.T) {
	const stat = "197 (bpfilter_umh) S 2 0 0 0 0 4194304"
	if !kernelChild(stat, true) {
		t.Fatal("kernel-created helper counted as agent job")
	}
	if kernelChild(stat, false) {
		t.Fatal("container PID 2 agent children were ignored")
	}
	if kernelChild("300 (bpfilter_umh) S 1 0 0 0 0 4194304", true) {
		t.Fatal("process name bypassed job monitor")
	}
}
