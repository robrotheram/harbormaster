package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestHealthyHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("unexpected health path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if !healthy(server.URL+"/healthz", "") {
		t.Fatal("HTTP health check failed")
	}
}

func TestHealthyUnixSocket(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			socket := filepath.Join(t.TempDir(), "controller.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			server.Listener.Close()
			server.Listener = listener
			server.Start()
			defer server.Close()
			if got := healthy("http://firecracker/healthz", socket); got != (status == http.StatusOK) {
				t.Fatalf("healthy = %v for status %d", got, status)
			}
		})
	}
}

func TestHealthyMissingUnixSocket(t *testing.T) {
	if healthy("http://firecracker/healthz", filepath.Join(t.TempDir(), "missing.sock")) {
		t.Fatal("missing socket reported healthy")
	}
}
