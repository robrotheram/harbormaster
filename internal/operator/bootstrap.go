package operator

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
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
	tlsDir := filepath.Join(local, "vault-tls")
	for _, dir := range []string{local, secrets, tlsDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return err
		}
	}
	for _, name := range []string{"control_token", "controller_token", "gateway_service_token", "grant_secret", "mcp_token", "postgres_password", "dashboard_local_password", "credential_encryption_key", "project_signing_key"} {
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
	if err = createOnce(filepath.Join(secrets, "subscription_vault_token"), nil, 0644); err != nil {
		return err
	}
	certPath, keyPath := filepath.Join(tlsDir, "cert.pem"), filepath.Join(secrets, "vault_tls_key")
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	if errors.Is(certErr, os.ErrNotExist) && errors.Is(keyErr, os.ErrNotExist) {
		key, err := rsa.GenerateKey(rand.Reader, 3072)
		if err != nil {
			return err
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			return err
		}
		cert := x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "vault"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(365 * 24 * time.Hour), DNSNames: []string{"vault", "localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
		der, err := x509.CreateCertificate(rand.Reader, &cert, &cert, &key.PublicKey, key)
		if err != nil {
			return err
		}
		if err = createOnce(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0644); err != nil {
			return err
		}
		if err = createOnce(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
			return err
		}
	} else if certErr != nil || keyErr != nil {
		return errors.New("incomplete or inaccessible Vault TLS material; restore the existing pair")
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
