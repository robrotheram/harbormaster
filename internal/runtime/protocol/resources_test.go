package protocol

import "testing"

func TestResourcesCannotSilentlyDivergeFromApprovedTemplate(t *testing.T) {
	approved := ResourceLimits{CPUCores: 1, MemoryBytes: 1 << 30}
	for _, tt := range []struct {
		name      string
		requested *ResourceLimits
		accepted  bool
	}{
		{"maintenance-default", nil, true},
		{"exact-template", &ResourceLimits{CPUCores: 1, MemoryBytes: 1 << 30}, true},
		{"under-accounted-memory", &ResourceLimits{CPUCores: 1, MemoryBytes: 512 << 20}, false},
		{"unsupported-memory", &ResourceLimits{CPUCores: 1, MemoryBytes: 2 << 30}, false},
		{"unsupported-cpu", &ResourceLimits{CPUCores: 2, MemoryBytes: 1 << 30}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckResources(tt.requested, approved) == nil; got != tt.accepted {
				t.Fatalf("accepted = %v, want %v", got, tt.accepted)
			}
		})
	}
}
