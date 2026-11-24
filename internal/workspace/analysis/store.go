package analysis

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"agent-workbench/internal/shared"
)

const maxEntries = 128
const maxStorage = 4 << 30
const maxResponse = shared.AnalysisResponseLimit
const maxSchema = 64 << 10
const maxArtifact = 32 << 10
const maxQueuedQueries = 4

//go:embed engine.py
var duckDBProgram string

type Store struct {
	dir, token, python      string
	uploads, artifacts      sync.Mutex
	querySlots, queryActive chan struct{}
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Dataset struct {
	ID      string   `json:"dataset_id"`
	Name    string   `json:"name"`
	Format  string   `json:"format"`
	Size    int64    `json:"size_bytes"`
	SHA256  string   `json:"sha256"`
	Columns []Column `json:"columns"`
}

func New(dataDir, token, python string) *Store {
	return &Store{
		dir: filepath.Join(dataDir, "analysis"), token: token, python: python,
		querySlots: make(chan struct{}, maxQueuedQueries), queryActive: make(chan struct{}, 1),
	}
}

func (s *Store) Register(register func(string, http.HandlerFunc)) {
	register("GET /v1/analysis/datasets", s.list)
	register("POST "+shared.AnalysisUploadPath, func(w http.ResponseWriter, r *http.Request) {
		s.upload(w, r, "dataset")
	})
	register("GET /v1/analysis/datasets/{dataset}/schema", s.schema)
	register("POST /v1/analysis/datasets/{dataset}/preview", s.previewDataset)
	register("GET /v1/analysis/artifacts/{artifact}", s.getArtifact)
	register("POST /v1/analysis/artifacts/{artifact}/data", s.artifactData)
	// The controller allowlist excludes local registration and result inspection.
	register("POST /v1/analysis/results/upload", func(w http.ResponseWriter, r *http.Request) {
		s.upload(w, r, "result")
	})
	register("GET /v1/analysis/results/{result}/schema", s.resultSchema)
	register("POST /v1/analysis/artifacts", s.createArtifact)
}

func (s *Store) open() (*os.Root, error) {
	parent, err := os.OpenRoot(filepath.Dir(s.dir))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	dir := filepath.Base(s.dir)
	if err = parent.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	st, err := parent.Lstat(dir)
	if err != nil || !st.IsDir() {
		return nil, os.ErrInvalid
	}
	return parent.OpenRoot(dir)
}

func openRegular(root *os.Root, name string) (*os.File, error) {
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Sys().(*syscall.Stat_t).Nlink != 1 {
		f.Close()
		return nil, os.ErrInvalid
	}
	return f, nil
}

func readNamedJSON(root *os.Root, name string, maxSize int64, result any) error {
	f, err := openRegular(root, name)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil || int64(len(b)) > maxSize {
		return os.ErrInvalid
	}
	return json.Unmarshal(b, result)
}

func readDataset(root *os.Root, id, kind string) (Dataset, error) {
	var dataset Dataset
	if !shared.ValidID(id) || !strings.HasPrefix(id, kind+"_") {
		return dataset, os.ErrNotExist
	}
	entry, err := root.Lstat(id)
	if err != nil || !entry.IsDir() {
		return dataset, os.ErrNotExist
	}
	err = readNamedJSON(root, path.Join(id, "metadata.json"), maxSchema, &dataset)
	if dataset.ID != id || dataset.Format != "csv" && dataset.Format != "parquet" {
		return Dataset{}, os.ErrNotExist
	}
	return dataset, err
}

func (s *Store) datasetRecords(root *os.Root, cleanup bool) ([]Dataset, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	items := []Dataset{}
	for {
		names, err := dir.Readdirnames(64)
		for _, id := range names {
			kind, _, _ := strings.Cut(id, "_")
			if cleanup && strings.HasPrefix(id, ".upload_") {
				_ = root.RemoveAll(id)
			}
			if kind == "dataset" || kind == "result" {
				if dataset, err := readDataset(root, id, kind); err == nil {
					items = append(items, dataset)
				}
			}
			if len(items) > maxEntries {
				return nil, os.ErrInvalid
			}
		}
		if err == io.EOF {
			return items, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (s *Store) list(w http.ResponseWriter, r *http.Request) {
	root, err := s.open()
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	defer root.Close()
	records, err := s.datasetRecords(root, false)
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	type summary struct {
		ID     string `json:"dataset_id"`
		Name   string `json:"name"`
		Format string `json:"format"`
		Size   int64  `json:"size_bytes"`
		SHA256 string `json:"sha256"`
	}
	items := []summary{}
	for _, item := range records {
		if strings.HasPrefix(item.ID, "dataset_") {
			items = append(items, summary{item.ID, item.Name, item.Format, item.Size, item.SHA256})
		}
	}
	shared.JSON(w, 200, map[string]any{"items": items})
}

func (s *Store) schema(w http.ResponseWriter, r *http.Request) {
	s.sourceSchema(w, r.PathValue("dataset"), "dataset")
}

func (s *Store) resultSchema(w http.ResponseWriter, r *http.Request) {
	s.sourceSchema(w, r.PathValue("result"), "result")
}

func (s *Store) sourceSchema(w http.ResponseWriter, id, kind string) {
	root, err := s.open()
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	defer root.Close()
	dataset, err := readDataset(root, id, kind)
	if err != nil {
		shared.Error(w, 404, "ANALYSIS_DATA_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, dataset)
}

func (s *Store) upload(w http.ResponseWriter, r *http.Request, kind string) {
	name := r.URL.Query()["name"]
	if len(name) != 1 || !shared.AnalysisFilename(name[0]) {
		shared.Error(w, 400, "INVALID_DATASET_UPLOAD")
		return
	}
	if !s.uploads.TryLock() {
		shared.Error(w, 429, "ANALYSIS_UPLOAD_BUSY")
		return
	}
	defer s.uploads.Unlock()
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(shared.AnalysisUploadTimeout))
	defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
	root, err := s.open()
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	defer root.Close()
	records, err := s.datasetRecords(root, true)
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	available := int64(maxStorage)
	for _, record := range records {
		available -= record.Size
	}
	if available <= 0 || len(records) >= maxEntries {
		shared.Error(w, 507, "ANALYSIS_STORAGE_LIMIT")
		return
	}
	limit := min(available, shared.AnalysisUploadLimit)
	if r.ContentLength > limit {
		shared.Error(w, 413, "DATASET_TOO_LARGE")
		return
	}
	id := shared.NewID(kind + "_")
	tmp := ".upload_" + id
	if err := root.Mkdir(tmp, 0700); err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	defer root.RemoveAll(tmp)
	format := strings.ToLower(path.Ext(name[0]))[1:]
	file := path.Join(tmp, "data."+format)
	f, err := root.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(r.Body, limit+1))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if size > limit {
		shared.Error(w, 413, "DATASET_TOO_LARGE")
		return
	}
	if err != nil || closeErr != nil || r.Context().Err() != nil {
		var bodyLimit *http.MaxBytesError
		if errors.As(err, &bodyLimit) {
			shared.Error(w, 413, "DATASET_TOO_LARGE")
		} else {
			shared.Error(w, 400, "ANALYSIS_UPLOAD_INCOMPLETE")
		}
		return
	}
	if size == 0 {
		shared.Error(w, 422, "INVALID_DATASET")
		return
	}
	dataset := Dataset{ID: id, Name: name[0], Format: format, Size: size, SHA256: hex.EncodeToString(hash.Sum(nil))}
	var result engineResult
	if !s.query(w, r, root, file, engineRequest{Format: format}, &result) {
		return
	}
	dataset.Columns = result.Columns
	data, err := json.Marshal(dataset)
	if err != nil || len(data) > maxSchema {
		shared.Error(w, 422, "ANALYSIS_SCHEMA_TOO_WIDE")
		return
	}
	if err = root.Chmod(file, 0400); err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	if err = root.WriteFile(path.Join(tmp, "metadata.json"), data, 0400); err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	if r.Context().Err() != nil {
		return
	}
	if err = root.Rename(tmp, id); err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 201, dataset)
}

type engineRequest struct {
	Format  string       `json:"format"`
	Scratch string       `json:"scratch"`
	Columns []Column     `json:"columns,omitempty"`
	Preview *dataRequest `json:"preview,omitempty"`
}

type engineResult struct {
	Columns   []Column          `json:"columns"`
	Rows      []json.RawMessage `json:"rows"`
	HasMore   bool              `json:"has_more"`
	Sampled   bool              `json:"sampled"`
	Truncated bool              `json:"truncated"`
	Error     string            `json:"error,omitempty"`
}

func (s *Store) query(w http.ResponseWriter, r *http.Request, root *os.Root, file string, request engineRequest, result *engineResult) bool {
	select {
	case s.querySlots <- struct{}{}:
		defer func() { <-s.querySlots }()
	default:
		shared.Error(w, 429, "ANALYSIS_QUERY_BUSY")
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	select {
	case s.queryActive <- struct{}{}:
		defer func() { <-s.queryActive }()
	case <-ctx.Done():
		shared.Error(w, 504, "ANALYSIS_QUERY_TIMEOUT")
		return false
	}
	source, err := openRegular(root, file)
	if err != nil {
		shared.Error(w, 404, "ANALYSIS_DATA_UNAVAILABLE")
		return false
	}
	defer source.Close()
	tmp := ".query_" + shared.NewID("")
	if err := root.Mkdir(tmp, 0700); err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return false
	}
	defer root.RemoveAll(tmp)
	request.Scratch = filepath.Join(s.dir, tmp)
	cmd := exec.CommandContext(ctx, s.python, "-I", "-c", duckDBProgram)
	cmd.Env = []string{"LANG=C.UTF-8", "HOME=" + request.Scratch, "TMPDIR=" + request.Scratch}
	cmd.Dir = request.Scratch
	cmd.ExtraFiles = []*os.File{source}
	body, _ := json.Marshal(request)
	cmd.Stdin = strings.NewReader(string(body))
	out := shared.NewCappedOutput(maxResponse)
	cmd.Stdout = out
	err = cmd.Run()
	switch {
	case ctx.Err() != nil:
		shared.Error(w, 504, "ANALYSIS_QUERY_TIMEOUT")
	case err != nil:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			shared.Error(w, 422, "ANALYSIS_QUERY_FAILED")
		} else {
			shared.Error(w, 503, "ANALYSIS_ENGINE_UNAVAILABLE")
		}
	case out.Overflow || json.Unmarshal(out.Data, result) != nil:
		shared.Error(w, 422, "ANALYSIS_RESPONSE_TOO_LARGE")
	case result.Error == "ANALYSIS_ENGINE_UNAVAILABLE":
		shared.Error(w, 503, result.Error)
	case result.Error != "":
		shared.Error(w, 422, result.Error)
	default:
		return true
	}
	return false
}
