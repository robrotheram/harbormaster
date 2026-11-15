package kubernetes_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the real shell guard with mock external programs. No cluster or
// database is contacted, and the mocks deliberately stop after the safety gate.
func TestBackupSuspensionGuard(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq required for operational guard tests")
	}
	for _, tc := range []struct {
		name, mode, status string
		observed           int
		allowed            bool
	}{
		{"active", "Running", "False", 4, false},
		{"stale", "Suspended", "True", 3, false},
		{"quiesced", "Suspended", "True", 4, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			fixture := filepath.Join(dir, "sandboxes.json")
			marker := filepath.Join(dir, "dump-attempted")
			data, _ := json.Marshal(map[string]any{"items": []any{map[string]any{"metadata": map[string]any{"generation": 4}, "spec": map[string]any{"operatingMode": tc.mode}, "status": map[string]any{"conditions": []any{map[string]any{"type": "Suspended", "status": tc.status, "observedGeneration": tc.observed}}}}}})
			writeTestFile(t, fixture, string(data), 0600)
			writeTestFile(t, filepath.Join(dir, "kubectl"), "#!/bin/sh\ncase \"$*\" in *'get sandboxes -o json'*) cat \"$FIXTURE\";; esac\n", 0700)
			writeTestFile(t, filepath.Join(dir, "pg_dump"), "#!/bin/sh\ntouch \"$MARKER\"\nexit 71\n", 0700)
			cmd := exec.Command("bash", "backup.sh")
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "FIXTURE="+fixture, "MARKER="+marker, "DATABASE_URL=unused", "PROJECT_NAMESPACE=probe", "SNAPSHOT_CLASS=probe", "BACKUP_DIR="+filepath.Join(dir, "backup"))
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("guard or mock should stop backup")
			}
			_, stat := os.Stat(marker)
			if (stat == nil) != tc.allowed {
				t.Fatalf("dump gate allowed=%v, want %v: %s", stat == nil, tc.allowed, output)
			}
		})
	}
}

func TestRecoveryRejectsUnsafePVCBeforeDatabaseRestore(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq required for operational guard tests")
	}
	for _, mode := range []string{"ReadWriteOnce", "ReadWriteOncePod"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "restore-attempted")
			manifest := filepath.Join(dir, "claims.json")
			writeTestFile(t, filepath.Join(dir, "metadata.dump"), "fixture", 0600)
			sum := sha256.Sum256([]byte("fixture"))
			writeTestFile(t, filepath.Join(dir, "SHA256SUMS"), fmt.Sprintf("%x  metadata.dump\n", sum), 0600)
			writeTestFile(t, manifest, `{"items":[{"kind":"PersistentVolumeClaim","metadata":{"labels":{"workbench.home":"home","workbench.project":"project"}},"spec":{"accessModes":["`+mode+`"],"dataSource":{"kind":"VolumeSnapshot"}}}]}`, 0600)
			writeTestFile(t, filepath.Join(dir, "kubectl"), "#!/bin/sh\nexit 0\n", 0700)
			writeTestFile(t, filepath.Join(dir, "pg_restore"), "#!/bin/sh\ntouch \"$MARKER\"\nexit 71\n", 0700)
			cmd := exec.Command("bash", "recover.sh")
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "MARKER="+marker, "BACKUP_DIR="+dir, "RECOVERY_DATABASE_URL=unused", "RESTORED_PVC_MANIFEST="+manifest)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("guard or mock should stop restore")
			}
			_, stat := os.Stat(marker)
			if (stat == nil) != (mode == "ReadWriteOncePod") {
				t.Fatalf("unexpected restore gate: %s", out)
			}
		})
	}
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(content, "\r\n", "\n")), mode); err != nil {
		t.Fatal(err)
	}
}
