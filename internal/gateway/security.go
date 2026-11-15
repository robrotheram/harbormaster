// Package gateway implements the inference gateway and shared gateway security.
package gateway

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"
)

var ErrDenied = errors.New("access denied")

type Identity struct {
	Connections *shared.ConnectionPolicy `json:"connections,omitempty"`
	WorkspaceID string                   `json:"workspace_id"`
	ProjectID   string                   `json:"project_id"`
	PrincipalID string                   `json:"principal_id"`
}
type Verifier func(context.Context, string, string) (Identity, error)

// NewVerifier trusts only the configured controller, never caller identity headers.
func NewVerifier(controllerURL, serviceToken string) (Verifier, error) {
	u, err := url.Parse(controllerURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || serviceToken == "" {
		return nil, ErrDenied
	}
	u.Path = "/v1/grants/verify"
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}, CheckRedirect: NoRedirect}
	return func(ctx context.Context, token, audience string) (Identity, error) {
		var id Identity
		body, _ := json.Marshal(map[string]string{"token": token, "audience": audience})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(string(body)))
		if err != nil {
			return id, ErrDenied
		}
		req.Header.Set("Authorization", "Bearer "+serviceToken)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return id, ErrDenied
		}
		defer res.Body.Close()
		b, err := ReadBounded(res.Body, 32<<10)
		if err != nil || res.StatusCode != http.StatusOK || json.Unmarshal(b, &id) != nil || !ValidIdentity(id) {
			return Identity{}, ErrDenied
		}
		return id, nil
	}, nil
}
func ValidIdentity(id Identity) bool {
	return id.WorkspaceID != "" && id.ProjectID != "" && id.PrincipalID != ""
}
func Authenticate(r *http.Request, verify Verifier, audience string) (Identity, error) {
	v := r.Header.Get("Authorization")
	if verify == nil || !strings.HasPrefix(v, "Bearer ") || len(v) > 8192 {
		return Identity{}, ErrDenied
	}
	token := strings.TrimPrefix(v, "Bearer ")
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return Identity{}, ErrDenied
	}
	id, err := verify(r.Context(), token, audience)
	if err != nil || !ValidIdentity(id) {
		return Identity{}, ErrDenied
	}
	return id, nil
}
func NoRedirect(*http.Request, []*http.Request) error { return ErrDenied }
func ReadBounded(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, ErrDenied
	}
	return b, nil
}
func ReadSecret(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", ErrDenied
	}
	defer f.Close()
	b, err := ReadBounded(f, 64<<10)
	if err != nil {
		return "", ErrDenied
	}
	s := strings.TrimSpace(string(b))
	if s == "" || strings.ContainsAny(s, "\r\n") {
		return "", ErrDenied
	}
	return s, nil
}
func ReadConfig(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		return ErrDenied
	}
	defer f.Close()
	b, err := ReadBounded(f, 1<<20)
	if err != nil {
		return err
	}
	return Decode(b, dst)
}
func Decode(b []byte, dst any) error {
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return ErrDenied
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return ErrDenied
	}
	return nil
}
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func Error(w http.ResponseWriter, status int, code string) {
	JSON(w, status, map[string]any{"code": code, "message": strings.ToLower(strings.ReplaceAll(code, "_", " ")), "retryable": status == 503})
}

// NetworkPolicy is operator controlled. Explicit CIDRs are required, including for
// private addresses. DNS is checked at dial time and only a checked IP is dialed.
// Proxy environment variables are intentionally ignored.
type NetworkPolicy struct {
	AllowedCIDRs []string `json:"allowed_cidrs"`
	CAFile       string   `json:"ca_file,omitempty"`
	AllowPrivate bool     `json:"allow_private,omitempty"`
}

func HTTPSOrigin(origin string) (*url.URL, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return nil, ErrDenied
	}
	u.Path = ""
	return u, nil
}
func (p NetworkPolicy) Transport(cert *tls.Certificate) (*http.Transport, error) {
	return p.transport(cert, net.DefaultResolver.LookupNetIP)
}

func (p NetworkPolicy) transport(cert *tls.Certificate, lookup func(context.Context, string, string) ([]netip.Addr, error)) (*http.Transport, error) {
	if len(p.AllowedCIDRs) == 0 {
		return nil, ErrDenied
	}
	prefixes := make([]netip.Prefix, 0, len(p.AllowedCIDRs))
	for _, raw := range p.AllowedCIDRs {
		v, err := netip.ParsePrefix(raw)
		if err != nil {
			return nil, ErrDenied
		}
		prefixes = append(prefixes, v)
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS12}
	if p.CAFile != "" {
		b, err := os.ReadFile(p.CAFile)
		if err != nil {
			return nil, ErrDenied
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(b) {
			return nil, ErrDenied
		}
		tc.RootCAs = pool
	}
	if cert != nil {
		tc.Certificates = []tls.Certificate{*cert}
	}
	allowed := func(ip netip.Addr) bool {
		ip = ip.Unmap()
		// Unspecified/multicast addresses are never valid service destinations.
		if ip.IsUnspecified() || ip.IsMulticast() {
			return false
		}
		if !p.AllowPrivate && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
			return false
		}
		for _, prefix := range prefixes {
			if prefix.Contains(ip) {
				return true
			}
		}
		return false
	}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, ErrDenied
		}
		ips, err := lookup(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, ErrDenied
		}
		d := net.Dialer{Timeout: 5 * time.Second}
		for _, ip := range ips {
			// A dual-stack DNS answer may include addresses outside the policy.
			// Dial only approved literals, without resolving the hostname again.
			if !allowed(ip) {
				continue
			}
			conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, ErrDenied
	}
	return &http.Transport{Proxy: nil, DialContext: dial, TLSClientConfig: tc, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxResponseHeaderBytes: 32 << 10, DisableKeepAlives: true}, nil
}
