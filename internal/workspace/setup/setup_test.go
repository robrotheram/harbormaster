package setup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testService(t *testing.T) Service {
	t.Helper()
	data := t.TempDir()
	s := Service{Data: data, Home: filepath.Join(data, "home"), Skills: true}
	if err := os.MkdirAll(s.project(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(Dir(s.Home), 0700); err != nil {
		t.Fatal(err)
	}
	git(t, s, "init", "-q", "-b", "main")
	return s
}

func git(t *testing.T, s Service, args ...string) {
	t.Helper()
	cmd := exec.Command("/usr/bin/git", append([]string{"-C", s.project()}, args...)...)
	cmd.Env = []string{"PATH=" + BasePath, "HOME=" + s.Home, "GIT_CONFIG_NOSYSTEM=1"}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git: %v %s", err, output)
	}
}

func file(t *testing.T, root, name, content string) {
	t.Helper()
	dest := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, root, name, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, name))
	if err != nil || string(content) != want {
		t.Fatalf("%s = %q, want %q (%v)", name, content, want, err)
	}
}

func TestApplyRejectsChangedCheckout(t *testing.T) {
	for _, change := range []string{"file", "branch"} {
		t.Run(change, func(t *testing.T) {
			s := testService(t)
			const original = "[tools]\nnode = \"24\"\n"
			file(t, s.project(), "mise.toml", original)
			root, err := os.OpenRoot(s.project())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			snapshot, err := s.fingerprint(context.Background(), root, toolInputs)
			if err != nil {
				t.Fatal(err)
			}
			stage := filepath.Join(Dir(s.Home), "plan-test")
			file(t, stage, "mise.toml", "[tools]\nnode = \"22\"\n")
			p := &Plan{stage: stage, snapshot: snapshot, operation: "tool-set", files: []string{"mise.toml"}}
			want := original
			if change == "file" {
				want = "[tools]\nnode = \"20\"\n"
				file(t, s.project(), "mise.toml", want)
			} else {
				git(t, s, "symbolic-ref", "HEAD", "refs/heads/another-branch")
			}
			if err := s.Apply(context.Background(), p); !errors.Is(err, ErrChanged) {
				t.Fatalf("apply: %v", err)
			}
			assertFile(t, s.project(), "mise.toml", want)
		})
	}
}

func TestRecoverInterruptedPublish(t *testing.T) {
	s := testService(t)
	stage := filepath.Join(Dir(s.Home), "plan-interrupted")
	original := "[tools]\nnode = \"24\"\n"
	file(t, stage, "backup-0", original)
	file(t, s.project(), "mise.toml", "[tools]\nnode = \"22\"\n")
	file(t, stage, "mise.lock", "never published")
	changes := []change{
		{Target: "mise.toml", Staged: "mise.toml", Original: true},
		{Target: "mise.lock", Staged: "mise.lock"},
	}
	raw, _ := json.Marshal(changes)
	file(t, stage, "journal.json", string(raw))
	if err := s.Recover(); err != nil {
		t.Fatal(err)
	}
	assertFile(t, s.project(), "mise.toml", original)
	if _, err := os.Stat(filepath.Join(s.project(), "mise.lock")); !os.IsNotExist(err) {
		t.Fatalf("published lockfile appeared during recovery: %v", err)
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("stage left behind: %v", err)
	}
}

func TestRemoveTrackedSkillPreservesLocalFilesAndMetadata(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	file(t, s.project(), skillLock, `{"version":2,"skills":{"tracked":{"source":"owner/repo"}},"metadata":{"team":"engineering"}}`)
	file(t, s.project(), ".agents/skills/tracked/SKILL.md", "tracked")
	file(t, s.project(), ".agents/skills/local/SKILL.md", "local")
	if _, err := s.Prepare(ctx, "test", Request{Operation: "skill-remove", Name: "local"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("untracked removal: %v", err)
	}
	p, err := s.Prepare(ctx, "test", Request{Operation: "skill-remove", Name: "tracked"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	assertFile(t, s.project(), ".agents/skills/tracked/SKILL.md", "tracked")
	if err := s.Apply(ctx, p); err != nil {
		t.Fatal(err)
	}
	assertFile(t, s.project(), ".agents/skills/local/SKILL.md", "local")
	if _, err := os.Stat(filepath.Join(s.project(), ".agents/skills/tracked")); !os.IsNotExist(err) {
		t.Fatalf("tracked skill remains: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(s.project(), skillLock))
	if err != nil || !strings.Contains(string(b), "engineering") || strings.Contains(string(b), "tracked") {
		t.Fatalf("unexpected lockfile: %s (%v)", b, err)
	}
}

func TestRejectsUnsupportedMiseConfiguration(t *testing.T) {
	s := testService(t)
	for _, content := range []string{
		"[hooks]\npostinstall = \"echo should-not-run\"\n",
		"[tools]\nnode = \"{{ env.VALUE }}\"\n",
		"[tools]\nnode = { version = \"24\", postinstall = \"echo nope\" }\n",
	} {
		file(t, s.project(), "mise.toml", content)
		_, err := s.Prepare(context.Background(), "test", Request{Operation: "tool-set", Name: "node", Version: "24"})
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("prepare %q: %v", content, err)
		}
	}
}

func TestMiseLockOnlyAcceptsMatchingPublisherAssets(t *testing.T) {
	template := `lockfile_version = 4
[[tools.node]]
version = "24.11.1"
backend = "core:node"
specifiers = ["24.11.1"]
[tools.node."platforms.linux-x64"]
url = "ASSET"
`
	for _, tc := range []struct {
		asset string
		valid bool
	}{
		{"https://nodejs.org/dist/v24.11.1/node-v24.11.1-linux-x64.tar.gz", true},
		{"https://nodejs.org/dist/v20.0.0/node-v20.0.0-linux-x64.tar.gz", false},
		{"https://other.example/dist/v24.11.1/node-v24.11.1-linux-x64.tar.gz", false},
		{"https://nodejs.org/dist/v24.11.1/node-v24.11.1-a/../../../other", false},
	} {
		err := validateToolLock([]byte(strings.ReplaceAll(template, "ASSET", tc.asset)))
		if (err == nil) != tc.valid {
			t.Errorf("checking %q: %v", tc.asset, err)
		}
	}
}
