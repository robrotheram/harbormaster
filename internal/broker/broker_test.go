package broker

import (
	"agent-workbench/internal/gateway"
	"agent-workbench/internal/shared"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func verify(_ context.Context, token, audience string) (gateway.Identity, error) {
	if audience == "broker" && token == "foreign-project" {
		return gateway.Identity{WorkspaceID: "ws-foreign", ProjectID: "foreign", PrincipalID: "alice"}, nil
	}
	if audience != "broker" || (token != "alice" && token != "bob" && token != "unconfigured") {
		return gateway.Identity{}, errors.New("denied")
	}
	return gateway.Identity{WorkspaceID: "ws-" + token, ProjectID: "default", PrincipalID: token}, nil
}
func fixtureConfig() Config {
	return Config{Mode: "fixture", Verify: verify,
		Operations: map[string]Operation{"lookup": {Parameters: map[string]Parameter{"record_id": {Type: "string", Required: true, MaxLength: 16}}}},
		Principals: map[string]PrincipalPolicy{
			"alice": {Operations: map[string]Permission{"lookup": {ApprovedCases: []string{"CASE-1"}, Projects: []string{"default"}, Certificate: "alice"}}},
			"bob":   {Operations: map[string]Permission{"lookup": {ApprovedCases: []string{"CASE-2"}, Projects: []string{"default"}, Certificate: "bob"}}},
		},
	}
}

const validBody = `{"case_id":"CASE-1","justification":"authorized investigation","parameters":{"record_id":"abc"}}`

func request(h http.Handler, operation, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/v1/operations/"+operation, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Principal-ID", "alice")
	r.Header.Set("X-Project-ID", "default")
	r.Header.Set("X-Workspace-ID", "ws-alice")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestFixturePolicies(t *testing.T) {
	h, err := NewHandler(fixtureConfig())
	if err != nil {
		t.Fatal(err)
	}
	a := request(h, "lookup", "alice", validBody)
	b := request(h, "lookup", "alice", validBody)
	if a.Code != 200 || a.Body.String() != b.Body.String() {
		t.Fatalf("fixture failed %d", a.Code)
	}

	// Decode with explicit JSON names to check that headers cannot choose identity.
	var fields map[string]any
	_ = json.Unmarshal(a.Body.Bytes(), &fields)
	if fields["mode"] != "fixture" || fields["synthetic"] != true || fields["principal_id"] != "alice" || fields["workspace_id"] != "ws-alice" {
		t.Fatal("fixture identity/label missing")
	}
	for _, tc := range []struct {
		name, operation, token, body string
		status                       int
	}{
		{"grant denial", "lookup", "bad", validBody, 401},
		{"operation denial", "unknown", "alice", validBody, 404},
		{"missing principal policy", "lookup", "unconfigured", validBody, 404},
		{"principal partition", "lookup", "bob", validBody, 404},
		{"unapproved case", "lookup", "alice", strings.Replace(validBody, "CASE-1", "CASE-3", 1), 404},
		{"spoofing body", "lookup", "alice", `{"case_id":"CASE-1","justification":"ok","parameters":{"record_id":"abc"},"principal_id":"bob"}`, 400},
		{"arbitrary URL", "lookup", "alice", `{"case_id":"CASE-1","justification":"ok","parameters":{"url":"https://evil.test"}}`, 400},
		{"missing parameter", "lookup", "alice", `{"case_id":"CASE-1","justification":"ok","parameters":{}}`, 400},
		{"wrong parameter type", "lookup", "alice", `{"case_id":"CASE-1","justification":"ok","parameters":{"record_id":true}}`, 400},
		{"missing parameters object", "lookup", "alice", `{"case_id":"CASE-1","justification":"ok"}`, 400},
		{"null parameter", "lookup", "alice", `{"case_id":"CASE-1","justification":"ok","parameters":{"record_id":null}}`, 400},
		{"overlong parameter", "lookup", "alice", strings.Replace(validBody, "abc", strings.Repeat("a", 17), 1), 400},
		{"trailing body", "lookup", "alice", validBody + `{}`, 400},
		{"oversized body", "lookup", "alice", strings.Repeat("a", (64<<10)+1), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if w := request(h, tc.operation, tc.token, tc.body); w.Code != tc.status {
				t.Fatalf("got %d want %d", w.Code, tc.status)
			}
		})
	}
	// Bob's valid case must produce Bob's controller identity despite Alice headers.
	w := request(h, "lookup", "bob", strings.Replace(validBody, "CASE-1", "CASE-2", 1))
	_ = json.Unmarshal(w.Body.Bytes(), &fields)
	if w.Code != 200 || fields["principal_id"] != "bob" || fields["workspace_id"] != "ws-bob" {
		t.Fatal("spoofed headers selected identity")
	}
}

type certificateAuthority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func makeCA(t *testing.T) certificateAuthority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return certificateAuthority{cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}
func (ca certificateAuthority) issue(t *testing.T, name string, server bool) (tls.Certificate, string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if server {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert, string(certPEM), string(keyPEM)
}
func policyForCA(t *testing.T, caPEM []byte) gateway.NetworkPolicy {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, caPEM, 0600); err != nil {
		t.Fatal(err)
	}
	return gateway.NetworkPolicy{AllowedCIDRs: []string{"127.0.0.1/32"}, AllowPrivate: true, CAFile: path}
}
func TestBrokerMTLSIdentityIsolation(t *testing.T) {
	ca := makeCA(t)
	serverCert, _, _ := ca.issue(t, "upstream", true)
	_, aliceCert, aliceKey := ca.issue(t, "alice", false)
	_, bobCert, bobKey := ca.issue(t, "bob", false)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca.pem)
	var mu sync.Mutex
	var identities []string
	var redirect atomic.Bool
	var redirectHits atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectHits.Add(1) }))
	defer target.Close()
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirect.Load() {
			http.Redirect(w, r, target.URL, 307)
			return
		}
		peer := r.TLS.PeerCertificates[0].Subject.CommonName
		mu.Lock()
		identities = append(identities, peer)
		mu.Unlock()
		if r.URL.Path != "/records/lookup" || r.Method != "POST" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected upstream route or headers")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["principal_id"] != peer || body["workspace_id"] != "ws-"+peer {
			t.Error("certificate/verified identity mismatch")
		}
		shared.JSON(w, 200, map[string]string{"identity": peer})
	}))
	upstream.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool, MinVersion: tls.VersionTLS12}
	upstream.StartTLS()
	defer upstream.Close()
	c := fixtureConfig()
	c.Mode = "upstream"
	c.certificateDir = t.TempDir()
	alicePEM := filepath.Join(c.certificateDir, "alice.pem")
	for path, pem := range map[string]string{
		alicePEM: aliceCert + aliceKey,
		filepath.Join(c.certificateDir, "bob.pem"): bobCert + bobKey,
	} {
		if err := os.WriteFile(path, []byte(pem), 0600); err != nil {
			t.Fatal(err)
		}
	}
	op := c.Operations["lookup"]
	op.Origin = upstream.URL
	op.Path = "/records/lookup"
	op.Method = "POST"
	op.Network = policyForCA(t, ca.pem)
	c.Operations["lookup"] = op
	h, err := NewHandler(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, principal := range []string{"alice", "bob", "alice"} {
		body := validBody
		if principal == "bob" {
			body = strings.Replace(body, "CASE-1", "CASE-2", 1)
		}
		w := request(h, "lookup", principal, body)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"identity":"`+principal+`"`) {
			t.Fatalf("mTLS failed for %s, status %d", principal, w.Code)
		}
	}
	if w := request(h, "lookup", "unconfigured", validBody); w.Code != 404 {
		t.Fatal("missing policy allowed")
	}
	if w := request(h, "lookup", "foreign-project", validBody); w.Code != 404 {
		t.Fatal("matching principal and case authorized a foreign project")
	}
	if err := os.Remove(alicePEM); err != nil {
		t.Fatal(err)
	}
	if w := request(h, "lookup", "alice", validBody); w.Code != 503 || strings.Contains(w.Body.String(), alicePEM) {
		t.Fatal("missing certificate failure was not sanitized")
	}
	if err := os.WriteFile(alicePEM, []byte(aliceCert+bobKey), 0600); err != nil {
		t.Fatal(err)
	}
	if w := request(h, "lookup", "alice", validBody); w.Code != 503 || strings.Contains(w.Body.String(), "PRIVATE KEY") {
		t.Fatal("mismatched key failure was not sanitized")
	}
	if err := os.WriteFile(alicePEM, []byte(aliceCert+aliceKey), 0600); err != nil {
		t.Fatal(err)
	}
	redirect.Store(true)
	if w := request(h, "lookup", "alice", validBody); w.Code != 503 || redirectHits.Load() != 0 {
		t.Fatal("upstream redirect followed")
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(identities, ",") != "alice,bob,alice" {
		t.Fatal("certificate identity leaked across requests")
	}
}
func TestBrokerRejectsInvalidCertificateNames(t *testing.T) {
	c := fixtureConfig()
	c.Mode = "upstream"
	op := c.Operations["lookup"]
	op.Origin = "https://example.test"
	op.Path = "/lookup"
	op.Method = "POST"
	op.Network = gateway.NetworkPolicy{AllowedCIDRs: []string{"203.0.113.7/32"}}
	c.Operations["lookup"] = op
	if _, err := NewHandler(c); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "../root", "%2e%2e", "alice.pem", "/alice"} {
		c.Principals["alice"] = PrincipalPolicy{Operations: map[string]Permission{"lookup": {ApprovedCases: []string{"CASE-1"}, Projects: []string{"default"}, Certificate: path}}}
		if _, err := NewHandler(c); err == nil {
			t.Fatal("invalid certificate name accepted")
		}
	}
	for _, projects := range [][]string{nil, {"*"}} {
		c.Principals["alice"] = PrincipalPolicy{Operations: map[string]Permission{"lookup": {ApprovedCases: []string{"CASE-1"}, Projects: projects, Certificate: "alice"}}}
		if _, err := NewHandler(c); err == nil {
			t.Fatal("upstream policy accepted projects", projects)
		}
	}
}

func TestConfigFromEnvExample(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "service-token")
	if err := os.WriteFile(tokenPath, []byte("test-service-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_CONFIG", "config.example.json")
	t.Setenv("GATEWAY_MODE", "fixture")
	t.Setenv("GATEWAY_SERVICE_TOKEN_FILE", tokenPath)
	t.Setenv("CONTROLLER_URL", "")
	c, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewHandler(c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_MODE", "upstream")
	if _, err := ConfigFromEnv(); err != nil {
		t.Fatal(err)
	}
}
