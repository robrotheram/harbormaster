package api

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFactoryProjectAPIExecutesRealRecipeAndFencesTemplates(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "factory-api")
	base := "/v1/workspaces/" + ws.ID + "/recipe-jobs"
	res := request(t, s, "POST", base, `{"template":"repository-manifest","idempotency_key":"forbidden"}`)
	if res.Code != 422 || rt.starts != 0 {
		t.Fatalf("unapproved template allocated: %d %s", res.Code, res.Body)
	}
	r := httptest.NewRequest("POST", base, strings.NewReader(`{"template":"research-v1","idempotency_key":"unauthorized"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("unauthorized job admitted")
	}
	template := "research-v1"
	budget := 600
	deadline := time.Now().Add(15 * time.Second)
	// Run this same API/workspace composition inside the trusted image to exercise
	// actual GoGallery admission, build/test, generation and evidence publication.
	if os.Getenv("FACTORY_LIVE_GALLERY") == "1" {
		template = "gogallery-v1"
		budget = 1800
		deadline = time.Now().Add(31 * time.Minute)
	}
	payload, _ := json.Marshal(map[string]any{"template": template, "timeout_seconds": budget, "idempotency_key": "platform-real-job"})
	res = request(t, s, "POST", base, string(payload))
	if res.Code != 202 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	var job struct {
		ID, State string
		ErrorCode string `json:"error_code"`
	}
	json.Unmarshal(res.Body.Bytes(), &job)
	for {
		res = request(t, s, "GET", base+"/"+job.ID, "")
		if res.Code != 200 {
			t.Fatalf("%d %s", res.Code, res.Body)
		}
		json.Unmarshal(res.Body.Bytes(), &job)
		if job.State != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if job.State != "succeeded" {
		t.Fatalf("job failed: %+v", job)
	}
	res = request(t, s, "POST", base, string(payload))
	if res.Code != 200 {
		t.Fatalf("job retry not idempotent: %d %s", res.Code, res.Body)
	}
	if os.Getenv("FACTORY_LIVE_GALLERY") != "1" {
		res = request(t, s, "POST", base, `{"template":"gogallery-v1","idempotency_key":"gallery-needs-image"}`)
		if _, err := os.Stat("/opt/workbench/gogallery/run"); err != nil && res.Code != 422 {
			t.Fatalf("missing trusted image did not fail admission: %d %s", res.Code, res.Body)
		}
	}
}

func TestFactoryAPIRetainsSnapshotAfterCodingStop(t *testing.T) { testFactoryAPIRetention(t, false) }
func TestFactoryAPIReconcilesExpiryAfterPlatformRestart(t *testing.T) {
	testFactoryAPIRetention(t, true)
}
func testFactoryAPIRetention(t *testing.T, expire bool) {
	s, rt := setup(t)
	coding := create(t, s, "coding-preview")
	retained := create(t, s, "retained-preview")
	res := request(t, s, "POST", "/v1/workspaces/"+coding.ID+"/start", `{"idempotency_key":"start-coding"}`)
	if res.Code != 200 {
		t.Fatalf("start %d %s", res.Code, res.Body)
	}
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	tw.WriteHeader(&tar.Header{Name: "site/index.html", Size: 14, Mode: 0600})
	tw.Write([]byte("<h1>hello</h1>"))
	tw.Close()
	if err := os.WriteFile(filepath.Join(rt.base, coding.ID, "files", "gallery.tar"), b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	status, body, err := rt.Agent(context.Background(), coding.ID, "POST", "/v1/artifacts", []byte(`{"source_path":"gallery.tar","idempotency_key":"gallery-archive"}`))
	if err != nil || status != 201 {
		t.Fatalf("archive %d %s %v", status, body, err)
	}
	var artifact struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &artifact)
	ttl := 3600
	if expire {
		ttl = 2
	}
	payload, _ := json.Marshal(map[string]any{"retained_workspace_id": retained.ID, "artifact_id": artifact.ID, "ttl_seconds": ttl})
	res = request(t, s, "POST", "/v1/workspaces/"+coding.ID+"/retained-previews", string(payload))
	if res.Code != 201 {
		t.Fatalf("retain %d %s", res.Code, res.Body)
	}
	var service struct {
		ID string `json:"id"`
	}
	json.Unmarshal(res.Body.Bytes(), &service)
	res = request(t, s, "POST", "/v1/workspaces/"+coding.ID+"/stop", `{"idempotency_key":"stop-coding"}`)
	if res.Code != 200 {
		t.Fatalf("stop %d %s", res.Code, res.Body)
	}
	base := "/v1/workspaces/" + retained.ID + "/services/" + service.ID
	res = request(t, s, "POST", base+"/fetch", `{"path":"index.html"}`)
	if res.Code != 200 {
		t.Fatalf("retained fetch %d %s", res.Code, res.Body)
	}
	if expire {
		time.Sleep(3 * time.Second)
		restarted := New(s.Store, s.Runtime, s.cfg)
		restarted.Reconcile(context.Background())
		saved, err := s.Store.Workspace(context.Background(), retained.ID)
		if err != nil || saved.State != "STOPPED" || saved.RetainedPreview == nil {
			t.Fatalf("expiry did not release confirmed compute: %+v %v", saved, err)
		}
		return
	}
	res = request(t, s, "POST", base+"/revoke", `{}`)
	if res.Code != 200 {
		t.Fatalf("revoke %d %s", res.Code, res.Body)
	}
	res = request(t, s, "POST", base+"/fetch", `{"path":"index.html"}`)
	if res.Code != 410 {
		t.Fatalf("revoked fetch %d", res.Code)
	}
	res = request(t, s, "POST", base+"/release", `{}`)
	if res.Code != 200 {
		t.Fatalf("release %d %s", res.Code, res.Body)
	}
	saved, err := s.Store.Workspace(context.Background(), retained.ID)
	if err != nil || saved.State != "STOPPED" {
		t.Fatalf("capacity not confirmed released: %+v %v", saved, err)
	}
}

func TestFactoryAPIProjectCredentialsCannotReachForeignEnvironments(t *testing.T) {
	s, rt := setup(t)
	a := domainProject(t, s, "factory-A")
	b := domainProject(t, s, "factory-B")
	s.cfg.ProjectTokens = map[string]string{"factory-a": a.ID, "factory-b": b.ID}
	foreign := domainWorkspace(t, s, "factory-b", b.ID, "foreign-factory")
	base := "/v1/workspaces/" + foreign.ID
	for _, tc := range []struct{ method, path, body string }{
		{"POST", base + "/recipe-jobs", `{"template":"gogallery-v1","idempotency_key":"forged"}`},
		{"GET", base + "/recipe-jobs/job_one", ""},
		{"POST", base + "/recipe-jobs/job_one/cancel", `{}`},
		{"POST", base + "/retained-previews", `{"retained_workspace_id":"ws_other","artifact_id":"artifact_one","ttl_seconds":60}`},
		{"GET", base + "/services/service_one", ""},
		{"POST", base + "/services/service_one/fetch", `{"path":"index.html"}`},
		{"POST", base + "/services/service_one/revoke", `{}`},
		{"POST", base + "/services/service_one/release", `{}`},
	} {
		res := domainRequest(t, s, "factory-a", tc.method, tc.path, tc.body)
		if res.Code != 404 {
			t.Fatalf("foreign route %s %s: %d", tc.method, tc.path, res.Code)
		}
	}
	if rt.starts != 0 {
		t.Fatal("foreign request allocated compute")
	}
}
