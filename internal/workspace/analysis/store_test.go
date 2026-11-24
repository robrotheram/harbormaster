package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func handler(t *testing.T, python string) (*Store, http.Handler) {
	t.Helper()
	s := New(t.TempDir(), "test-token", python)
	mux := http.NewServeMux()
	s.Register(func(pattern string, handler http.HandlerFunc) { mux.HandleFunc(pattern, handler) })
	return s, mux
}

func duckPython(t *testing.T) string {
	t.Helper()
	python := os.Getenv("WORKBENCH_ANALYSIS_PYTHON")
	if python != "" {
		if err := exec.Command(python, "-I", "-c", "import duckdb").Run(); err != nil {
			t.Fatal(err)
		}
		return python
	}
	python, err := exec.LookPath("python3")
	if err != nil || exec.Command(python, "-I", "-c", "import duckdb").Run() != nil {
		t.Skip("DuckDB unavailable. Set WORKBENCH_ANALYSIS_PYTHON to the Python interpreter with DuckDB installed.")
	}
	return python
}

func call(t *testing.T, h http.Handler, method, route string, body io.Reader, status int) []byte {
	t.Helper()
	r := httptest.NewRequest(method, "/v1/analysis"+route, body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s: got %d, want %d: %s", method, route, w.Code, status, w.Body.String())
	}
	return w.Body.Bytes()
}

func decoded[T any](t *testing.T, b []byte) T {
	t.Helper()
	var data T
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestArtifactCatalogAndDeletion(t *testing.T) {
	s, h := handler(t, "")
	root, err := s.open()
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	const source = "result_squirrels"
	if err := root.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(source+"/metadata.json", []byte(`{"dataset_id":"result_squirrels","format":"parquet","columns":[{"name":"count","type":"BIGINT"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	const spec = `{"schema_version":"1.0","artifact_id":"squirrel_counts","type":"table","title":"Counts","data":{"source_type":"query_result","reference":"result_squirrels"},"visualization":{"columns":[{"field":"count"}]}}`
	call(t, h, "POST", "/artifacts", strings.NewReader(spec), 201)
	call(t, h, "POST", "/artifacts", strings.NewReader(strings.Replace(spec, "squirrel_counts", "other_counts", 1)), 201)
	list := func() []byte { return call(t, h, "GET", "/artifacts", nil, 200) }
	if b := list(); !bytes.Contains(b, []byte("squirrel_counts")) || bytes.Contains(b, []byte(source)) {
		t.Fatalf("unexpected catalog: %s", b)
	}
	for range 2 {
		call(t, h, "POST", "/artifacts/squirrel_counts/delete", strings.NewReader("{}"), 200)
	}
	call(t, h, "GET", "/artifacts/squirrel_counts", nil, 404)
	call(t, h, "POST", "/artifacts/squirrel_counts/data", strings.NewReader("{}"), 404)
	call(t, h, "GET", "/artifacts/other_counts", nil, 200)
	call(t, h, "GET", "/results/"+source+"/schema", nil, 200)
	if b := list(); bytes.Contains(b, []byte("squirrel_counts")) || !bytes.Contains(b, []byte("other_counts")) {
		t.Fatalf("unexpected catalog after deletion: %s", b)
	}
}

func TestCSVPreviewAndArtifactIsolation(t *testing.T) {
	python := duckPython(t)
	s, h := handler(t, python)
	const input = "x,y,group\n2,4,a\n1,3,b\n1,,a\n3,8,a\n"
	metadata := decoded[Dataset](t, call(t, h, "POST", "/datasets/upload?name=sensor.csv", strings.NewReader(input), 201))
	if metadata.Format != "csv" || metadata.Size != int64(len(input)) || len(metadata.SHA256) != 64 || len(metadata.Columns) != 3 {
		t.Fatalf("invalid dataset: %+v", metadata)
	}
	if listing := call(t, h, "GET", "/datasets", nil, 200); !bytes.Contains(listing, []byte(metadata.ID)) || bytes.Contains(listing, []byte(s.dir)) {
		t.Fatalf("invalid listing: %s", listing)
	}
	route := "/datasets/" + metadata.ID + "/preview"
	type response struct {
		engineResult
		Cursor string `json:"next_cursor"`
	}
	req := Preview{Limit: 2, Columns: []string{"x", "y"}, Sort: []Sort{{"x", "asc"}}}
	preview := func(req Preview, status int) []byte {
		t.Helper()
		b, _ := json.Marshal(req)
		return call(t, h, "POST", route, bytes.NewReader(b), status)
	}
	first := decoded[response](t, preview(req, 200))
	if len(first.Rows) != 2 || string(first.Rows[0]) != "[1,3]" || string(first.Rows[1]) != "[1,null]" || !first.HasMore || first.Cursor == "" {
		t.Fatalf("bad first page: %+v", first)
	}
	req.Cursor = first.Cursor
	next := decoded[response](t, preview(req, 200))
	if len(next.Rows) != 2 || string(next.Rows[0]) != "[2,4]" || next.HasMore || next.Cursor != "" {
		t.Fatalf("bad next page: %+v", next)
	}
	req.Filters = []Filter{{"group", "eq", "a"}}
	preview(req, 400)
	req.Cursor = ""
	filtered := decoded[response](t, preview(req, 200))
	if len(filtered.Rows) != 2 || string(filtered.Rows[0]) != "[1,null]" || string(filtered.Rows[1]) != "[2,4]" {
		t.Fatalf("bad filtered page: %+v", filtered)
	}
	req.Columns = []string{`x"); SELECT 1 --`}
	if b := preview(req, 422); !bytes.Contains(b, []byte("ANALYSIS_UNKNOWN_FIELD")) {
		t.Fatal("unknown column accepted")
	}
	if b := call(t, h, "POST", route, strings.NewReader(`{"sql":"SELECT *"}`), 400); !bytes.Contains(b, []byte("INVALID_REQUEST")) {
		t.Fatal("SQL accepted")
	}
	call(t, h, "POST", "/datasets/upload?name=../private.csv", strings.NewReader(input), 400)
	call(t, h, "POST", "/datasets/upload?name=wrong.parquet", strings.NewReader(input), 422)

	table := `{"schema_version":"1.0","artifact_id":"table_001","type":"table","title":"Sensor rows","data":{"source_type":"dataset","reference":"` + metadata.ID + `"},"visualization":{"page_size":2,"columns":[{"field":"x"},{"field":"y"}],"sortable":true}}`
	reg := "/artifacts"
	call(t, h, "POST", reg, strings.NewReader(table), 201)
	call(t, h, "POST", reg, strings.NewReader(table), 200)
	call(t, h, "POST", reg, strings.NewReader(strings.Replace(table, "Sensor rows", "New title", 1)), 409)
	call(t, h, "POST", reg, strings.NewReader(strings.Replace(table, `"1.0"`, `"2.0"`, 1)), 422)
	call(t, h, "POST", reg, strings.NewReader(strings.Replace(table, `"sortable":true`, `"script":"<script>"`, 1)), 422)
	call(t, h, "POST", reg, strings.NewReader(strings.Replace(table, `"field":"y"`, `"field":"unknown"`, 1)), 422)
	call(t, h, "GET", "/artifacts/table_001", nil, 200)
	call(t, h, "POST", "/artifacts/table_001/data", strings.NewReader(`{"filters":[{"field":"x","op":"eq","value":1}]}`), 400)
	b := call(t, h, "POST", "/artifacts/table_001/data", strings.NewReader("{}"), 200)
	if len(decoded[response](t, b).Rows) != 2 {
		t.Fatal("table ignored page size")
	}

	pie := `{"schema_version":"1.0","artifact_id":"counts_by_group","type":"chart","title":"Counts by group","data":{"source_type":"dataset","reference":"` + metadata.ID + `"},"visualization":{"chart_type":"pie","encoding":{"category":"group","value":"x"},"style":{"category_colors":{"a":"#4cc5ad"}}}}`
	call(t, h, "POST", reg, strings.NewReader(pie), 201)
	b = call(t, h, "POST", "/artifacts/counts_by_group/data", strings.NewReader("{}"), 200)
	result := decoded[engineResult](t, b)
	if len(result.Columns) != 2 || result.Columns[0].Name != "group" || result.Columns[1].Name != "x" ||
		len(result.Rows) != 4 || result.Truncated || result.Sampled || result.HasMore {
		t.Fatalf("pie did not receive its complete projected data: %s", b)
	}

	heatmap := `{"schema_version":"1.0","artifact_id":"density","type":"geo","title":"Density","data":{"source_type":"dataset","reference":"` + metadata.ID + `"},"visualization":{"geometry":"point","layer":"heatmap","radius_px":35,"coordinate_system":{"type":"geographic","crs":"EPSG:4326"},"encoding":{"x":"x","y":"y","weight":"x"},"view":{"mode":"map"}}}`
	call(t, h, "POST", reg, strings.NewReader(strings.Replace(heatmap, `"weight":"x"`, `"weight":"group"`, 1)), 422)
	call(t, h, "POST", reg, strings.NewReader(strings.Replace(heatmap, `"radius_px":35`, `"radius_px":101`, 1)), 422)
	call(t, h, "POST", reg, strings.NewReader(heatmap), 201)
	b = call(t, h, "POST", "/artifacts/density/data", strings.NewReader("{}"), 200)
	if data := decoded[response](t, b); len(data.Rows) != 4 || data.Sampled {
		t.Fatalf("unexpected heatmap data: %s", b)
	}

	grouped := strings.NewReplacer(`"artifact_id":"density"`, `"artifact_id":"grouped-density"`,
		`"weight":"x"`, `"weight":"x","group":"group"`).Replace(heatmap)
	call(t, h, "POST", reg, strings.NewReader(grouped), 201)
	b = call(t, h, "POST", "/artifacts/grouped-density/data", strings.NewReader("{}"), 200)
	if data := decoded[response](t, b); len(data.Columns) != 3 || data.Columns[2].Name != "group" {
		t.Fatalf("heatmap group field is missing: %s", b)
	}

	for _, config := range []struct {
		layer  string
		option string
	}{
		{"grid", `"cell_size_m":100`},
		{"hexagon", `"radius_m":100`},
		{"contour", `"cell_size_m":100,"thresholds":[1,5]`},
		{"screen_grid", `"cell_size_px":24`},
	} {
		spec := strings.NewReplacer(`"artifact_id":"density"`, `"artifact_id":"`+config.layer+`"`,
			`"layer":"heatmap","radius_px":35`, `"layer":"`+config.layer+`",`+config.option).Replace(heatmap)
		call(t, h, "POST", reg, strings.NewReader(spec), 201)
		call(t, h, "POST", reg, strings.NewReader(strings.Replace(spec, config.option, `"radius_px":35`, 1)), 422)
	}
	for _, geometry := range []string{"path", "polygon", "geojson"} {
		spec := `{"schema_version":"1.0","artifact_id":"` + geometry + `","type":"geo","title":"Shape","data":{"source_type":"dataset","reference":"` + metadata.ID + `"},"visualization":{"geometry":"` + geometry + `","coordinate_system":{"type":"geographic","crs":"EPSG:4326"},"encoding":{"shape":"group"},"view":{"mode":"map"}}}`
		call(t, h, "POST", reg, strings.NewReader(spec), 201)
		call(t, h, "POST", reg, strings.NewReader(strings.Replace(spec, `"shape":"group"`, `"shape":"x"`, 1)), 422)
		b := call(t, h, "POST", "/artifacts/"+geometry+"/data", strings.NewReader("{}"), 200)
		if data := decoded[response](t, b); len(data.Columns) != 1 || data.Columns[0].Name != "group" {
			t.Fatalf("expected projection of only the shape field: %s", b)
		}
	}

	_, foreign := handler(t, python)
	call(t, foreign, "GET", "/datasets/"+metadata.ID+"/schema", nil, 404)
	call(t, foreign, "GET", "/artifacts/table_001", nil, 404)
	call(t, foreign, "POST", reg, strings.NewReader(table), 404)
	dataFile := filepath.Join(s.dir, metadata.ID, "data.csv")
	if err := os.Remove(dataFile); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private.csv")
	if err := os.WriteFile(outside, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, dataFile); err != nil {
		t.Fatal(err)
	}
	call(t, h, "POST", "/artifacts/table_001/data", strings.NewReader("{}"), 404)
	if err := os.RemoveAll(s.dir); err != nil {
		t.Fatal(err)
	}
	call(t, h, "GET", "/artifacts/table_001", nil, 404)
}

func TestLargeParquetAndDerivedArtifacts(t *testing.T) {
	python := duckPython(t)
	s, h := handler(t, python)
	temp := t.TempDir()
	filename := filepath.Join(temp, "sensor.parquet")
	program := `
import duckdb, sys
c = duckdb.connect(config={"threads": 1})
c.execute("""COPY (SELECT i AS row_id, hash(i) AS sensor, i%360-180 AS longitude,
    i%150-75 AS latitude, CASE WHEN i%997=0 THEN 1000 ELSE i%42 END AS temperature,
    repeat(md5(i::VARCHAR), 140) AS payload
    FROM range(850000) t(i)) TO ? (FORMAT parquet)""", [sys.argv[1]])
`
	if out, err := exec.Command(python, "-I", "-c", program, filename).CombinedOutput(); err != nil {
		t.Fatalf("prepare Parquet fixture: %v %s", err, out)
	}
	upload := func(endpoint, path string) Dataset {
		t.Helper()
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		return decoded[Dataset](t, call(t, h, "POST", endpoint+"/upload?name="+filepath.Base(path), f, 201))
	}
	source := upload("/datasets", filename)
	route := "/datasets/" + source.ID + "/preview"
	b := call(t, h, "POST", route, strings.NewReader(`{"columns":["row_id","temperature"],"limit":100}`), 200)
	data := decoded[engineResult](t, b)
	if len(data.Rows) != 100 || !data.HasMore || len(b) >= 8192 || source.Size < 8<<20 {
		t.Fatalf("unexpected file or preview size. Source %d, response %d, rows %d", source.Size, len(b), len(data.Rows))
	}
	b = call(t, h, "POST", route, strings.NewReader(`{"columns":["payload"],"limit":500}`), 200)
	data = decoded[engineResult](t, b)
	if len(b) > maxResponse || len(data.Rows) >= 500 || !data.Truncated || !data.HasMore {
		t.Fatalf("byte limit failed. Response %d, rows %d", len(b), len(data.Rows))
	}
	call(t, h, "POST", route, strings.NewReader(`{"limit":501}`), 400)

	counts := filepath.Join(temp, "histogram.parquet")
	aggregate := `
import duckdb, sys
c = duckdb.connect(config={"threads": 1})
c.read_parquet(sys.argv[1]).create_view("sensor")
c.execute("""COPY (SELECT (temperature // 10) * 10 AS bin, count(*) AS count
    FROM sensor GROUP BY bin ORDER BY bin)
    TO ? (FORMAT parquet)""", [sys.argv[2]])
`
	workerPath := filepath.Join(s.dir, source.ID, "data.parquet")
	if out, err := exec.Command(python, "-I", "-c", aggregate, workerPath, counts).CombinedOutput(); err != nil {
		t.Fatalf("aggregate: %v %s", err, out)
	}
	result := upload("/results", counts)
	chart := `{"schema_version":"1.0","artifact_id":"anomalies","type":"chart","title":"Anomalies","data":{"source_type":"query_result","reference":"` + result.ID + `"},"visualization":{"chart_type":"histogram","encoding":{"x":{"field":"bin","type":"quantitative"},"y":{"field":"count","type":"quantitative"}}}}`
	call(t, h, "POST", "/artifacts", strings.NewReader(chart), 201)
	b = call(t, h, "POST", "/artifacts/anomalies/data", strings.NewReader("{}"), 200)
	data = decoded[engineResult](t, b)
	if len(data.Rows) != 6 || !strings.Contains(string(data.Rows[len(data.Rows)-1]), "1000") || data.Truncated || data.Sampled {
		t.Fatalf("anomaly bins: %s", b)
	}

	geo := `{"schema_version":"1.0","artifact_id":"locations","type":"geo","title":"Sensor locations","data":{"source_type":"dataset","reference":"` + source.ID + `"},"visualization":{"geometry":"point","coordinate_system":{"type":"geographic","crs":"EPSG:4326"},"encoding":{"x":"longitude","y":"latitude","color":"temperature"},"view":{"mode":"map","auto_fit":true}}}`
	call(t, h, "POST", "/artifacts", strings.NewReader(geo), 201)
	b = call(t, h, "POST", "/artifacts/locations/data", strings.NewReader("{}"), 200)
	data = decoded[engineResult](t, b)
	if len(data.Rows) != maxPoints || !data.Sampled || len(b) > maxResponse {
		t.Fatalf("map sample: size %d, rows %d, sampled %t", len(b), len(data.Rows), data.Sampled)
	}
}

func TestQueryCancellationReleasesSlot(t *testing.T) {
	python := filepath.Join(t.TempDir(), "engine")
	// Replace only the subprocess in this test so cancellation is deterministic.
	if err := os.WriteFile(python, []byte("#!/bin/sh\n: > started\nexec sleep 60\n"), 0700); err != nil {
		t.Fatal(err)
	}
	s, _ := handler(t, python)
	root, err := s.open()
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.WriteFile("test.csv", []byte("x\n1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	done := make(chan bool, 1)
	go func() {
		done <- s.query(w, req, root, "test.csv", engineRequest{Format: "csv"}, &engineResult{})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		started, _ := filepath.Glob(filepath.Join(s.dir, ".query_*", "started"))
		if len(started) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("query did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	for range maxQueuedQueries - 1 {
		s.querySlots <- struct{}{}
	}
	busy := httptest.NewRecorder()
	if s.query(busy, req, root, "test.csv", engineRequest{}, &engineResult{}) || busy.Code != 429 {
		t.Fatalf("concurrent query admitted: %d", busy.Code)
	}
	cancel()
	select {
	case ok := <-done:
		if ok || w.Code != 504 {
			t.Fatalf("query survived cancellation: %d", w.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("query process was not interrupted")
	}
	if len(s.querySlots) != maxQueuedQueries-1 || len(s.queryActive) != 0 {
		t.Fatal("query slot stayed occupied")
	}
}
