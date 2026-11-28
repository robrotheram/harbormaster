package operator

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func createOnce(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	_, e := f.Write(data)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	return closeErr
}
func Bootstrap(root string, out io.Writer) error {
	local := filepath.Join(root, ".local")
	secrets := filepath.Join(local, "secrets")
	for _, dir := range []string{local, secrets} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return err
		}
	}
	for _, name := range []string{"control_token", "controller_token", "gateway_service_token", "subscription_gateway_token", "grant_secret", "mcp_token", "postgres_password", "dashboard_local_password", "dashboard_local_operator_token", "dashboard_read_token", "credential_encryption_key", "project_signing_key"} {
		value := make([]byte, 32)
		if _, err := rand.Read(value); err != nil {
			return err
		}
		if err := createOnce(filepath.Join(secrets, name), []byte(hex.EncodeToString(value)), 0644); err != nil {
			return err
		}
	}
	password, err := os.ReadFile(filepath.Join(secrets, "postgres_password"))
	if err != nil {
		return err
	}
	database := url.URL{Scheme: "postgres", User: url.UserPassword("workbench", strings.TrimSpace(string(password))), Host: "postgres:5432", Path: "/workbench", RawQuery: "sslmode=disable"}
	if err = createOnce(filepath.Join(secrets, "database_url"), []byte(database.String()), 0644); err != nil {
		return err
	}
	if err = bootstrapResolverTLS(secrets); err != nil {
		return err
	}
	example, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		return err
	}
	if err = createOnce(filepath.Join(root, ".env"), example, 0600); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "Local credentials provisioned; existing credentials preserved.")
	return err
}

func bootstrapResolverTLS(dir string) error {
	bundlePath := filepath.Join(dir, "subscription_resolver_tls")
	caPath := filepath.Join(dir, "subscription_resolver_ca")
	bundle, err := os.ReadFile(bundlePath)
	if errors.Is(err, os.ErrNotExist) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		now := time.Now()
		template := &x509.Certificate{
			DNSNames:    []string{"controlplane"},
			NotBefore:   now.Add(-time.Minute),
			NotAfter:    now.AddDate(1, 0, 0),
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
		if err != nil {
			return err
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return err
		}
		bundle = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		bundle = append(bundle, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})...)
		if err := createOnce(bundlePath, bundle, 0644); err != nil {
			return err
		}
		bundle, err = os.ReadFile(bundlePath)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	pair, err := tls.X509KeyPair(bundle, bundle)
	if err != nil {
		return errors.New("invalid subscription resolver TLS bundle")
	}
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]})
	if err := createOnce(caPath, ca, 0644); err != nil {
		return err
	}
	pinned, err := os.ReadFile(caPath)
	if err != nil || !bytes.Equal(pinned, ca) {
		return errors.New("subscription resolver TLS bundle does not match its pinned certificate")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || !time.Now().Before(leaf.NotAfter) {
		return errors.New("subscription resolver TLS certificate expired")
	}
	return nil
}
