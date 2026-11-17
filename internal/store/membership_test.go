package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentRevocationsCannotRemoveBothAdministrators(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "members.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	members := []ProjectMember{{ProjectID: "default", Email: "alice@example.com", UserID: "user_alice", Role: "admin", Status: "active"}, {ProjectID: "default", Email: "bob@example.com", UserID: "user_bob", Role: "admin", Status: "active"}}
	for _, m := range members {
		if err = s.SaveMember(ctx, m, 0, "", true); err != nil {
			t.Fatal(err)
		}
	}
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for _, m := range members {
		wait.Add(1)
		go func(m ProjectMember) {
			defer wait.Done()
			m.Status = "revoked"
			results <- s.SaveMember(ctx, m, 1, "", true)
		}(m)
	}
	wait.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if err != ErrMembershipConflict {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("unexpected successful revocations", success)
	}
	roster, err := s.ProjectMembers(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, m := range roster {
		if m.Status == "active" && m.Role == "admin" {
			active++
		}
	}
	if active != 1 {
		t.Fatal("last admin lost", roster)
	}
}
