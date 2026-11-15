package operator

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapPreservesCredentialsAndTLS(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("EXAMPLE=true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(root, io.Discard); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(root, ".local/secrets/control_token")
	token, _ := os.ReadFile(tokenPath)
	if len(token) != 64 {
		t.Fatal("invalid token")
	}
	if err := Bootstrap(root, io.Discard); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(tokenPath)
	if !bytes.Equal(token, again) {
		t.Fatal("rotated existing credentials")
	}
	if _, err := tls.LoadX509KeyPair(filepath.Join(root, ".local/vault-tls/cert.pem"), filepath.Join(root, ".local/secrets/vault_tls_key")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{".local", ".local/secrets", ".local/vault-tls"} {
		info, err := os.Stat(filepath.Join(root, dir))
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatal("unprotected directory", dir)
		}
	}
}
func TestControlClientDoesNotFollowRedirectsOrReflectSecrets(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	os.WriteFile(tokenFile, []byte("private-test-token"), 0600)
	t.Setenv("CONTROL_TOKEN_FILE", tokenFile)
	requests := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-test-token" {
			t.Error("missing auth")
		}
		w.Header().Set("Location", destination.URL)
		w.WriteHeader(302)
		io.WriteString(w, "private-test-token")
	}))
	defer source.Close()
	client, err := newClient(source.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.call(context.Background(), "GET", "/v1/workspaces", nil)
	if err == nil || strings.Contains(err.Error(), "private-test-token") || requests != 0 {
		t.Fatal("unsafe redirect or error", err)
	}
	for _, origin := range []string{"http://example.com", "https://user:password@example.com", "https://example.com/path"} {
		if _, err := newClient(origin); err == nil {
			t.Fatal("unsafe origin", origin)
		}
	}
}
func TestPrivateWriteDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original")
	target := filepath.Join(dir, "credential")
	os.WriteFile(original, []byte("unchanged"), 0644)
	os.Symlink(original, target)
	if err := privateWrite(target, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(original)
	if string(data) != "unchanged" {
		t.Fatal("followed symlink")
	}
	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0600 {
		t.Fatal("unsafe permissions")
	}
}
