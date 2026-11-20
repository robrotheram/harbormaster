package gateway

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestTransportFiltersDNSAddresses(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		addresses []string
		private   bool
		lookupErr error
		wantOK    bool
	}{
		{name: "IPv6 before approved IPv4", addresses: []string{"2606:4700:4408::ac40:9bd1", "127.0.0.1"}, private: true, wantOK: true},
		{name: "IPv6 after approved IPv4", addresses: []string{"127.0.0.1", "2606:4700:4408::ac40:9bd1"}, private: true, wantOK: true},
		{name: "unapproved IPv4 before approved IPv4", addresses: []string{"192.0.2.1", "127.0.0.1"}, private: true, wantOK: true},
		{name: "only unapproved addresses", addresses: []string{"192.0.2.1", "::1"}, private: true},
		{name: "private address still denied", addresses: []string{"127.0.0.1"}},
		{name: "no addresses", private: true},
		{name: "DNS error", addresses: []string{"127.0.0.1"}, private: true, lookupErr: errors.New("DNS failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			policy := NetworkPolicy{AllowedCIDRs: []string{"127.0.0.1/32"}, AllowPrivate: tc.private}
			transport, err := policy.transport(nil, func(_ context.Context, network, host string) ([]netip.Addr, error) {
				calls++
				if network != "ip" || host != "upstream.invalid" {
					t.Fatalf("unexpected lookup: %s %s", network, host)
				}
				var addresses []netip.Addr
				for _, address := range tc.addresses {
					addresses = append(addresses, netip.MustParseAddr(address))
				}
				return addresses, tc.lookupErr
			})
			if err != nil {
				t.Fatal(err)
			}
			defer transport.CloseIdleConnections()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			conn, err := transport.DialContext(ctx, "tcp", net.JoinHostPort("upstream.invalid", port))
			if conn != nil {
				conn.Close()
			}
			if (err == nil) != tc.wantOK {
				t.Fatalf("connection error = %v, want success %v", err, tc.wantOK)
			}
			if !tc.wantOK && !errors.Is(err, ErrDenied) {
				t.Fatalf("expected policy denial, got %v", err)
			}
			if calls != 1 {
				t.Fatalf("resolved %d times", calls)
			}
		})
	}
}

func TestTransportHostnamePolicy(t *testing.T) {
	calls := 0
	policy := NetworkPolicy{AllowedHosts: []string{"UPSTREAM.example."}, AllowPrivate: true}
	transport, err := policy.transport(nil, func(_ context.Context, _, host string) ([]netip.Addr, error) {
		calls++
		if host != "upstream.example" {
			t.Errorf("unexpected lookup for %s", host)
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"other.example", "upstream.example"} {
		if _, err := transport.DialContext(context.Background(), "tcp", host+":443"); !errors.Is(err, ErrDenied) {
			t.Fatalf("dial %s: %v", host, err)
		}
	}
	if calls != 1 {
		t.Fatalf("expected only the approved name to reach DNS, got %d lookups", calls)
	}
}

func TestPublicDestinationExcludesSpecialUseAddresses(t *testing.T) {
	for _, address := range []string{
		"100.64.0.1", "192.0.0.1", "198.18.0.1", "::ffff:100.64.0.1",
		"64:ff9b::a9fe:a9fe", "2002::1", "::192.0.2.1", "fec0::1", "3fff::1",
	} {
		if publicDestination(netip.MustParseAddr(address).Unmap()) {
			t.Errorf("special-use address accepted: %s", address)
		}
	}
	for _, address := range []string{"8.8.8.8", "2606:4700::1111", "::ffff:8.8.8.8"} {
		if !publicDestination(netip.MustParseAddr(address).Unmap()) {
			t.Errorf("public address rejected: %s", address)
		}
	}
}
