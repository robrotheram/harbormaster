package workspacetools

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ExperimentResult struct {
	SchemaVersion  int     `json:"schema_version"`
	Generator      string  `json:"generator"`
	Seed           uint64  `json:"seed"`
	Regularisation float64 `json:"regularisation"`
	DatasetSHA256  string  `json:"dataset_sha256"`
	TrainSamples   int     `json:"train_samples"`
	TestSamples    int     `json:"test_samples"`
	Slope          float64 `json:"slope"`
	MSE            float64 `json:"held_out_mse"`
	Elapsed        float64 `json:"elapsed_seconds"`
}

func Experiment(seed uint64, regularisation float64) (ExperimentResult, error) {
	if math.IsNaN(regularisation) || math.IsInf(regularisation, 0) || regularisation < 0 {
		return ExperimentResult{}, errors.New("regularisation must be finite and nonnegative")
	}
	rng := rand.New(rand.NewPCG(seed, 0))
	data := make([][2]float64, 256)
	for i := range data {
		data[i] = [2]float64{rng.Float64()*2 - 1, rng.NormFloat64() * 0.1}
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return ExperimentResult{}, err
	}
	sum := sha256.Sum256(encoded)
	started := time.Now()
	numerator, denominator := 0.0, regularisation
	for _, row := range data[:192] {
		x, noise := row[0], row[1]
		numerator += x * (2*x + noise)
		denominator += x * x
	}
	slope := numerator / denominator
	mse := 0.0
	for _, row := range data[192:] {
		residual := slope*row[0] - (2*row[0] + row[1])
		mse += residual * residual
	}
	return ExperimentResult{2, "go-pcg-normal-v1", seed, regularisation, hex.EncodeToString(sum[:]), 192, 64, slope, mse / 64, time.Since(started).Seconds()}, nil
}
func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}
func Research(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("research", flag.ContinueOnError)
	flags.SetOutput(out)
	seed := flags.Uint64("seed", 42, "dataset seed")
	regularisation := flags.Float64("regularisation", 0, "nonnegative regularisation")
	output := flags.String("output", "baseline.json", "output file")
	pair := flags.Bool("pair", false, "write baseline and regularised variation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected research arguments")
	}
	result, err := Experiment(*seed, *regularisation)
	if err != nil {
		return err
	}
	if err = writeJSON(*output, result); err != nil {
		return err
	}
	if *pair {
		variation, err := Experiment(*seed, 10)
		if err != nil {
			return err
		}
		return writeJSON("variation.json", variation)
	}
	return nil
}
func GalleryFixture(directory string) error {
	root, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return err
	}
	if err = os.Mkdir(root, 0700); err != nil {
		return err
	}
	for _, spec := range []struct {
		name          string
		width, height int
	}{{"album/landscape.png", 96, 64}, {"album/nested/portrait.png", 64, 96}, {"square.png", 80, 80}} {
		path := filepath.Join(root, "photos", spec.name)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		picture := image.NewNRGBA(image.Rect(0, 0, spec.width, spec.height))
		for y := 0; y < spec.height; y++ {
			for x := 0; x < spec.width; x++ {
				picture.SetNRGBA(x, y, color.NRGBA{64, 128, uint8(y), 255})
			}
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		encodeErr := png.Encode(file, picture)
		closeErr := file.Close()
		if encodeErr != nil {
			return encodeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	photos, _ := json.Marshal(filepath.Join(root, "photos"))
	site, _ := json.Marshal(filepath.Join(root, "site"))
	config := fmt.Sprintf("ui:\n  aienabled: false\ngallery:\n  name: Synthetic gallery\n  basepath: %s\n  destpath: %s\n  url: http://localhost:8080\n  theme: EmeraldNoir\n  imagesPerPage: 50\n  useoriginal: false\n", photos, site)
	return os.WriteFile(filepath.Join(root, "gallery.yaml"), []byte(config), 0600)
}
func Gallery(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) != 0 {
		return errors.New("gogallery recipe takes no arguments")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err = os.CopyFS("source", os.DirFS("/opt/workbench/gogallery/source")); err != nil {
		return err
	}
	if err = filepath.WalkDir("source", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("source symlinks are not supported")
		}
		return os.Chmod(path, info.Mode().Perm()|0200)
	}); err != nil {
		return err
	}
	env := []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GOCACHE=") && !strings.HasPrefix(value, "GOMODCACHE=") {
			env = append(env, value)
		}
	}
	env = append(env, "GOCACHE="+filepath.Join(cwd, "cache"), "GOMODCACHE=/opt/workbench/gogallery/modules")
	run := func(dir, name string, args ...string) error {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		cmd.Env = env
		cmd.Stdout = out
		cmd.Stderr = errOut
		return cmd.Run()
	}
	if err = run("source", "go", "test", "./..."); err != nil {
		return err
	}
	if err = run("source", "go", "build", "-o", "../gogallery", "."); err != nil {
		return err
	}
	if err = GalleryFixture("fixture"); err != nil {
		return err
	}
	if err = run("fixture", filepath.Join(cwd, "gogallery"), "--config", "gallery.yaml", "build"); err != nil {
		return err
	}
	file, err := os.OpenFile("gallery.tar", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	writer := tar.NewWriter(file)
	html := false
	err = filepath.WalkDir("fixture/site", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return errors.New("unexpected nonregular gallery output")
		}
		relative, err := filepath.Rel("fixture", path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err = writer.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		html = html || strings.HasSuffix(path, ".html")
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		_, err = io.Copy(writer, input)
		return err
	})
	tarErr := writer.Close()
	fileErr := file.Close()
	if err != nil {
		return err
	}
	if tarErr != nil {
		return tarErr
	}
	if fileErr != nil {
		return fileErr
	}
	if !html {
		return errors.New("no gallery HTML produced")
	}
	return writeJSON("provenance.json", map[string]any{"source_revision": "5f1bfd6a41afec65bc7ab791a14863180d944ee0", "template": "gogallery-v1", "checks": []string{"go test ./...", "go build", "synthetic gallery build"}})
}
