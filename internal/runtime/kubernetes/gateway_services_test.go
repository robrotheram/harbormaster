package kubernetes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverGateways(t *testing.T) {
	const prefix = "/api/v1/namespaces/custom-core/services/"
	addresses := map[string]string{
		prefix + "inference-gateway": "10.96.0.12",
		prefix + "credential-broker": "10.96.0.13",
		prefix + "egress-gateway":    "fd00::14",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, ok := addresses[r.URL.Path]
		if !ok || r.Method != http.MethodGet {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, `{"spec":{"clusterIP":%q}}`, ip)
	}))
	defer server.Close()
	k := &Kubernetes{cfg: KubernetesConfig{CoreNamespace: "custom-core", APIURL: server.URL}, apiHTTP: server.Client()}
	if err := k.discoverGateways(context.Background()); err != nil {
		t.Fatal(err)
	}
	if k.cfg.InferenceURL != "http://10.96.0.12:8080" || k.cfg.BrokerURL != "http://10.96.0.13:8080" || k.cfg.EgressURL != "http://[fd00::14]:8080" {
		t.Fatalf("wrong gateway URLs: %s, %s, %s", k.cfg.InferenceURL, k.cfg.BrokerURL, k.cfg.EgressURL)
	}
}
