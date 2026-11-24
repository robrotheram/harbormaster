package firecracker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFirecrackerUIDSkipsAllOverlappingSubordinateRanges(t *testing.T) {
	dir := t.TempDir()
	subuid, subgid := filepath.Join(dir, "subuid"), filepath.Join(dir, "subgid")
	if err := os.WriteFile(subuid, []byte("user:200005:10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subgid, []byte("user:200000:9\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if uid, err := reserveHostUID(firstGuestUID, subuid, subgid); err != nil || uid != 200015 {
		t.Fatalf("reserved UID %d: %v", uid, err)
	}
}
