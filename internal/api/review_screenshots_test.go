package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-workbench/internal/shared"
)

func TestReviewScreenshotsDeliveredAndRetried(t *testing.T) {
	s, rt := setup(t)
	project := domainProject(t, s, "screenshots")
	var item shared.WorkItem
	created := request(t, s, "POST", "/v1/work-items", `{"project_id":"`+project.ID+`","instruction":"Review this UI","kind":"software","idempotency_key":"screenshots"}`)
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &item) != nil {
		t.Fatal(created.Body.String())
	}
	root := "/v1/work-items/" + item.ID
	if got := request(t, s, "POST", root+"/run", `{"project_id":"`+project.ID+`","idempotency_key":"run"}`); got.Code != 202 {
		t.Fatal(got.Body.String())
	}
	item = waitWorkReview(t, s, project.ID, item.ID)
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	shot := reviewScreenshot{Content: imageBytes.Bytes(), Note: "Fix the spacing"}
	payload := map[string]any{"project_id": project.ID, "instruction": "Please fix this", "expected_attempt_id": item.LatestAttemptID, "idempotency_key": "feedback", "screenshots": []reviewScreenshot{shot}}
	body, _ := json.Marshal(payload)
	var next shared.WorkAttempt
	for _, expected := range []int{202, 200} {
		got := request(t, s, "POST", root+"/continue", string(body))
		if got.Code != expected || json.Unmarshal(got.Body.Bytes(), &next) != nil {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	files, err := filepath.Glob(filepath.Join(rt.base, item.WorkspaceID, "files", "work-requests", "*-screenshot-1.png"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	saved, err := os.ReadFile(files[0])
	if err != nil || !bytes.Equal(saved, shot.Content) {
		t.Fatal("screenshot bytes changed", err)
	}
	instruction, err := os.ReadFile(filepath.Join(rt.base, item.WorkspaceID, "files", next.SpecificationPath))
	if err != nil || !strings.Contains(string(instruction), filepath.Base(files[0])) || !strings.Contains(string(instruction), shot.Note) {
		t.Fatal("missing screenshot context", err)
	}
	shot.Note = "changed"
	payload["screenshots"] = []reviewScreenshot{shot}
	body, _ = json.Marshal(payload)
	if got := request(t, s, "POST", root+"/continue", string(body)); got.Code != 409 {
		t.Fatal("changed retry accepted", got.Code)
	}
}

func TestReviewScreenshotsValidation(t *testing.T) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	good := reviewScreenshot{Content: buf.Bytes(), Note: "note"}
	if !validReviewScreenshots([]reviewScreenshot{good}) {
		t.Fatal("valid image rejected")
	}
	for _, shots := range [][]reviewScreenshot{
		{{Content: []byte("<svg/>")}},
		{{Content: make([]byte, shared.HandoffLimit+1)}},
		{{Content: buf.Bytes(), Note: strings.Repeat("x", 2001)}},
		{good, good, good, good, good},
	} {
		if validReviewScreenshots(shots) {
			t.Fatal("invalid screenshots accepted")
		}
	}
}
