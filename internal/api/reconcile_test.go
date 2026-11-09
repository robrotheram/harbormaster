package api

import (
	"context"
	"testing"
)

type reconcilingRuntime struct {
	*embeddedRuntime
	reconciliations int
}

func (r *reconcilingRuntime) Reconcile(context.Context, string) error {
	r.reconciliations++
	return nil
}

func TestStartReadyWorkspaceReconcilesWithoutRestarting(t *testing.T) {
	s, rt := setup(t)
	reconciler := &reconcilingRuntime{embeddedRuntime: rt}
	s.Runtime = reconciler
	ws := create(t, s, "reconcile-existing")
	for i := 0; i < 2; i++ {
		w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"reconcile-start"}`)
		if w.Code != 200 {
			t.Fatalf("start failed: %d", w.Code)
		}
	}
	if rt.starts != 1 || reconciler.reconciliations != 1 {
		t.Fatal("ready workspace bypassed reconciliation or restarted")
	}
}
