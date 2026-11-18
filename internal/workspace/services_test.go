package workspace

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestRetainedSnapshotSurvivesCodingAndRetainedRestart(t *testing.T) {
	coding, err := New(Config{ID: "ws_coding", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{ID: "ws_preview", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"}
	retained, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	tw.WriteHeader(&tar.Header{Name: "site/index.html", Mode: 0600, Size: 14})
	tw.Write([]byte("<h1>hello</h1>"))
	tw.Close()
	payload, _ := json.Marshal(map[string]any{"snapshot": b.Bytes(), "ttl_seconds": 3600})
	res := invoke(t, retained.Handler(), "POST", "/v1/services", string(payload))
	if res.Code != 201 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	var s retainedService
	json.Unmarshal(res.Body.Bytes(), &s)
	coding.Close()
	retained.Close()
	retained, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Close()
	fetch := func(want int) {
		t.Helper()
		res := invoke(t, retained.Handler(), "POST", "/v1/services/"+s.ID+"/fetch", `{"path":"index.html"}`)
		if res.Code != want {
			t.Fatalf("%d %s", res.Code, res.Body)
		}
	}
	fetch(200)
	invoke(t, retained.Handler(), "POST", "/v1/services/"+s.ID+"/revoke", `{}`)
	fetch(410)
	// Expiration is enforced both during access and by reconciliation, keeping bytes.
	retained.db.Exec(`UPDATE retained_services SET state='active',expires_at=? WHERE id=?`, time.Now().Unix()-1, s.ID)
	invoke(t, retained.Handler(), "POST", "/v1/services/reconcile", `{}`)
	fetch(410)
	var size int
	retained.db.QueryRow(`SELECT length(snapshot) FROM retained_services WHERE id=?`, s.ID).Scan(&size)
	if size == 0 {
		t.Fatal("expiration erased archived evidence")
	}
	retained.db.Exec(`UPDATE retained_services SET state='active',expires_at=? WHERE id=?`, time.Now().Unix()-1, s.ID)
	fetch(410)
}

func TestRetainedPreviewBackgroundExpiryReleasesIdleReservation(t *testing.T) {
	m, err := New(Config{ID: "ws_expiry", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	_, err = m.db.Exec(`INSERT INTO retained_services VALUES('service_expiring','active',?,'{}')`, time.Now().Unix()+3600)
	if err != nil {
		t.Fatal(err)
	}
	if m.Activity().BackgroundJobs != 1 {
		t.Fatal("service failed to reserve compute")
	}
	m.db.Exec(`UPDATE retained_services SET expires_at=?`, time.Now().Unix()-1)
	deadline := time.Now().Add(3 * time.Second)
	for {
		var state string
		m.db.QueryRow(`SELECT state FROM retained_services WHERE id='service_expiring'`).Scan(&state)
		if state == "expired" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("automatic reconciliation did not expire service")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if m.Activity().BackgroundJobs != 0 {
		t.Fatal("expired service retained idle reservation")
	}
}

func TestRetainedPreviewRejectsHostTargetsAndArchiveLinks(t *testing.T) {
	m, err := New(Config{ID: "ws_fenced", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	for _, h := range []*tar.Header{
		{Name: "../index.html", Size: 1, Mode: 0600},
		{Name: "site/index.html", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		{Name: "site/index.html", Typeflag: tar.TypeLink, Linkname: "/etc/passwd"},
	} {
		var b bytes.Buffer
		tw := tar.NewWriter(&b)
		tw.WriteHeader(h)
		if h.Size > 0 {
			tw.Write([]byte("x"))
		}
		tw.Close()
		payload, _ := json.Marshal(map[string]any{"snapshot": b.Bytes(), "ttl_seconds": 3600})
		res := invoke(t, m.Handler(), "POST", "/v1/services", string(payload))
		if res.Code != 400 {
			t.Fatalf("unsafe archive admitted: %d", res.Code)
		}
	}
	res := invoke(t, m.Handler(), "POST", "/v1/services", `{"target":"http://169.254.169.254/","ttl_seconds":3600}`)
	if res.Code < 400 {
		t.Fatal("host target admitted")
	}
}

func TestRetainedReceiptsSurviveRestartAndRejectChangedPayload(t *testing.T) {
	cfg := Config{ID: "ws_receipts", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	tw.WriteHeader(&tar.Header{Name: "index.html", Mode: 0600, Size: 2})
	tw.Write([]byte("ok"))
	tw.Close()
	payload, _ := json.Marshal(map[string]any{"snapshot": b.Bytes(), "ttl_seconds": 60, "idempotency_key": "delivery-one"})
	first := invoke(t, m.Handler(), "POST", "/v1/services", string(payload))
	if first.Code != 201 {
		t.Fatalf("%d %s", first.Code, first.Body)
	}
	m.Close()
	m, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	replay := invoke(t, m.Handler(), "POST", "/v1/services", string(payload))
	if replay.Code != 201 || first.Body.String() != replay.Body.String() {
		t.Fatalf("lost ACK replay: %d %s", replay.Code, replay.Body)
	}
	changed, _ := json.Marshal(map[string]any{"snapshot": b.Bytes(), "ttl_seconds": 61, "idempotency_key": "delivery-one"})
	if res := invoke(t, m.Handler(), "POST", "/v1/services", string(changed)); res.Code != 409 {
		t.Fatalf("changed TTL %d", res.Code)
	}
	changed, _ = json.Marshal(map[string]any{"snapshot": b.Bytes(), "ttl_seconds": 60, "idempotency_key": "delivery-two"})
	if res := invoke(t, m.Handler(), "POST", "/v1/services", string(changed)); res.Code != 409 {
		t.Fatalf("untracked active service %d", res.Code)
	}
}
