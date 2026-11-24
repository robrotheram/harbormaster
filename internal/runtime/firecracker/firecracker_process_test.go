package firecracker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirecrackerJailCleanupWaitsForTermination(t *testing.T) {
	const contents = "persistent workspace data"
	persistent := filepath.Join(t.TempDir(), "data.ext4")
	if err := os.WriteFile(persistent, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	f := testFC()
	f.jailParent = t.TempDir()
	generation := "fc-" + strings.Repeat("a", 32)
	v := &fcVM{ID: "ws_cleanup", Generation: generation, running: true, Jail: filepath.Join(f.jailParent, generation, "root")}
	f.vms[v.ID] = v
	if err := os.MkdirAll(v.Jail, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(persistent, filepath.Join(v.Jail, "data.ext4")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v.Jail, "config.ext4"), []byte("ephemeral"), 0600); err != nil {
		t.Fatal(err)
	}
	f.terminateCgroup = func(string) error { return errors.New("busy") }
	if err := f.kill(v); err == nil {
		t.Fatal("unconfirmed termination accepted")
	}
	if _, err := os.Stat(v.Jail); err != nil || f.workspace(v.ID) != v {
		t.Fatalf("unconfirmed VM lost its jail or tracking: %v", err)
	}
	f.terminateCgroup = func(string) error { return nil }
	if err := f.kill(v); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(v.Jail)); !os.IsNotExist(err) || f.workspace(v.ID) != nil {
		t.Fatalf("confirmed teardown retained its jail or tracking: %v", err)
	}
	if body, err := os.ReadFile(persistent); err != nil || string(body) != contents {
		t.Fatalf("persistent disk changed: %v", err)
	}
}

func TestFirecrackerStartupReapsOnlyOwnedGenerationJails(t *testing.T) {
	groups, jails := t.TempDir(), t.TempDir()
	generation := "fc-" + strings.Repeat("b", 32)
	for _, name := range []string{generation, "unrelated"} {
		if err := os.Mkdir(filepath.Join(jails, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	terminations := 0
	if err := reapGenerations(groups, jails, func(group string) error {
		terminations++
		if group != filepath.Join(groups, generation) {
			t.Fatalf("unexpected cgroup: %s", group)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(jails)
	if err != nil || len(entries) != 1 || entries[0].Name() != "unrelated" || terminations != 1 {
		t.Fatalf("unexpected entries or cleanup: %v %v (%d terminations)", entries, err, terminations)
	}
}
