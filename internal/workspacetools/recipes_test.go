package workspacetools

import (
	"encoding/json"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestResearchReproducibleAndBounded(t *testing.T) {
	a, err := Experiment(42, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Experiment(42, 0)
	if err != nil {
		t.Fatal(err)
	}
	a.Elapsed, b.Elapsed = 0, 0
	if a != b || a.SchemaVersion != 2 || a.MSE > .05 {
		t.Fatalf("invalid reproducible baseline: %+v %+v", a, b)
	}
	variation, _ := Experiment(42, 10)
	if variation.DatasetSHA256 != a.DatasetSHA256 || variation.Slope >= a.Slope {
		t.Fatal("variation changed dataset or failed to regularise")
	}
	different, _ := Experiment(43, 0)
	if different.DatasetSHA256 == a.DatasetSHA256 {
		t.Fatal("seed ignored")
	}
	for _, value := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := Experiment(42, value); err == nil {
			t.Fatal("invalid parameter admitted")
		}
	}
	out := filepath.Join(t.TempDir(), "baseline.json")
	if err := Research([]string{"--output", out}, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var result ExperimentResult
	if json.Unmarshal(data, &result) != nil || result.DatasetSHA256 != a.DatasetSHA256 {
		t.Fatal("invalid output")
	}
}
func TestGalleryFixtureAndExistingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fixture")
	if err := GalleryFixture(dir); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(dir, "photos/album/nested/portrait.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	picture, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if picture.Bounds().Dx() != 64 || picture.Bounds().Dy() != 96 {
		t.Fatal("invalid portrait")
	}
	if err := GalleryFixture(dir); err == nil {
		t.Fatal("overwrote existing fixture")
	}
}
