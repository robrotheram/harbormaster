package gateway

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestLeaseRenewsWhileNoInferenceRequestsArrive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		maintainLease(ctx, time.Millisecond, func(context.Context) error {
			if calls.Add(1) == 3 {
				cancel()
			}
			return nil
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("lease maintenance did not stop")
	}
	if calls.Load() != 3 {
		t.Fatal("no idle renewal")
	}
}
