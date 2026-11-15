package api

import (
	"agent-workbench/internal/dashboard"
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFactoryProjectAPIExecutesRealRecipeAndFencesTemplates(t *testing.T) {
	if os.Getenv("FACTORY_LIVE_GALLERY") != "1" {
		dir := t.TempDir()
		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "workbench-tools"), "../../cmd/workbench-tools")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build recipe: %v %s", err, output)
		}
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

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

	// Retry after a simulated lost acknowledgement must return the same service.
	retry := request(t, s, "POST", "/v1/workspaces/"+coding.ID+"/retained-previews", string(payload))
	var replay struct {
		ID string `json:"id"`
	}
	json.Unmarshal(retry.Body.Bytes(), &replay)
	if retry.Code != 201 || replay.ID != service.ID {
		t.Fatalf("promotion replay: %d %s", retry.Code, retry.Body)
	}
	upstream := httptest.NewServer(s.Handler())
	defer upstream.Close()
	ingress, err := dashboard.NewInteractiveHandler(upstream.URL, upstream.URL, "control-token")
	if err != nil {
		t.Fatal(err)
	}
	invokeBrowser := func(method, target, host, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Host = host
		if method == "POST" {
			req.Header.Set("Origin", "http://"+host)
			req.Header.Set("X-Workbench-Request", "1")
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		out := httptest.NewRecorder()
		ingress.ServeHTTP(out, req)
		return out
	}
	issued := invokeBrowser("POST", "/ui-api/workspaces/"+retained.ID+"/services/"+service.ID+"/connect", "localhost:8088", "{}", nil)
	var link struct {
		URL string `json:"preview_url"`
	}
	json.Unmarshal(issued.Body.Bytes(), &link)
	u, err := url.Parse(link.URL)
	if err != nil || u.Host == "" {
		t.Fatalf("attachment %d %s", issued.Code, issued.Body)
	}
	ticket, _ := url.ParseQuery(u.Fragment)
	connectBody, _ := json.Marshal(map[string]string{"ticket": ticket.Get("ticket")})
	connected := invokeBrowser("POST", "/connect", u.Host, string(connectBody), nil)
	if connected.Code != 200 || len(connected.Result().Cookies()) != 1 {
		t.Fatalf("connect %d %s", connected.Code, connected.Body)
	}
	cookie := connected.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.Domain != "" || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe preview cookie")
	}
	if again := invokeBrowser("POST", "/connect", u.Host, string(connectBody), nil); again.Code != 403 {
		t.Fatal("ticket reused")
	}
	if foreign := invokeBrowser("GET", "/", "preview-foreign.localhost:8088", "", cookie); foreign.Code != 401 {
		t.Fatal("session crossed origins")
	}
	res = request(t, s, "POST", "/v1/workspaces/"+coding.ID+"/stop", `{"idempotency_key":"stop-coding"}`)
	if res.Code != 200 {
		t.Fatalf("stop %d %s", res.Code, res.Body)
	}
	replayAfterStop := request(t, s, "POST", "/v1/workspaces/"+coding.ID+"/retained-previews", string(payload))
	if replayAfterStop.Code != 201 {
		t.Fatalf("lost ACK after coding stopped: %d %s", replayAfterStop.Code, replayAfterStop.Body)
	}
	base := "/v1/workspaces/" + retained.ID + "/services/" + service.ID
	res = request(t, s, "POST", base+"/fetch", `{"path":"index.html"}`)
	if res.Code != 200 {
		t.Fatalf("retained fetch %d %s", res.Code, res.Body)
	}

	rendered := invokeBrowser("GET", "/", u.Host, "", cookie)
	if rendered.Code != 200 || rendered.Body.String() != "<h1>hello</h1>" {
		t.Fatalf("isolated render after coding stop: %d %s", rendered.Code, rendered.Body)
	}
	if strings.Contains(rendered.Body.String(), "control-token") {
		t.Fatal("credential leaked")
	}
	if expire {
		time.Sleep(3 * time.Second)
		if expired := invokeBrowser("GET", "/", u.Host, "", cookie); expired.Code != 401 {
			t.Fatalf("expired browser %d", expired.Code)
		}
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
	if revoked := invokeBrowser("GET", "/", u.Host, "", cookie); revoked.Code != 410 {
		t.Fatalf("revoked browser %d", revoked.Code)
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

func TestFactoryEvidenceSurvivesColdStorage(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "cold-factory-evidence")
	base := "/v1/workspaces/" + ws.ID
	request(t, s, "POST", base+"/start", `{"idempotency_key":"cold-start"}`)
	if err := os.WriteFile(filepath.Join(rt.base, ws.ID, "files", "evidence.txt"), []byte("retained evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	status, b, err := rt.Agent(t.Context(), ws.ID, "POST", "/v1/artifacts", []byte(`{"source_path":"evidence.txt","idempotency_key":"cold-evidence"}`))
	if err != nil || status != 201 {
		t.Fatalf("%d %s %v", status, b, err)
	}
	var artifact struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &artifact); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/artifacts", "/recipe-jobs", "/artifacts/" + artifact.ID + "/content"} {
		if res := request(t, s, "POST", base+"/stop", `{"idempotency_key":"cold-stop"}`); res.Code != 200 {
			t.Fatalf("stop %d %s", res.Code, res.Body)
		}
		if res := request(t, s, "GET", base+path, ""); res.Code != 200 {
			t.Fatalf("cold %s: %d %s", path, res.Code, res.Body)
		}
	}
}
