package analysis

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"

	"agent-workbench/internal/shared"
	"github.com/google/jsonschema-go/jsonschema"
)

//go:embed artifact-v1.schema.json
var artifactSchema []byte

var resolveSchema = sync.OnceValues(func() (*jsonschema.Resolved, error) {
	var schema jsonschema.Schema
	if err := json.Unmarshal(artifactSchema, &schema); err != nil {
		return nil, err
	}
	return schema.Resolve(nil)
})

type artifact struct {
	ID   string `json:"artifact_id"`
	Type string `json:"type"`
	Data struct {
		Type      string `json:"source_type"`
		Reference string `json:"reference"`
	} `json:"data"`
	Visualization json.RawMessage `json:"visualization"`
}

type tableSpec struct {
	PageSize int `json:"page_size"`
	Columns  []struct {
		Field string `json:"field"`
	} `json:"columns"`
	Sortable   bool `json:"sortable"`
	Filterable bool `json:"filterable"`
}

type axis struct {
	Field string `json:"field"`
	Type  string `json:"type"`
}

type chartSpec struct {
	ChartType string `json:"chart_type"`
	Encoding  struct {
		X        axis   `json:"x"`
		Y        axis   `json:"y"`
		Series   string `json:"series"`
		Category string `json:"category"`
		Value    string `json:"value"`
	} `json:"encoding"`
	Style struct {
		CategoryColors map[string]string `json:"category_colors"`
		SeriesColors   map[string]string `json:"series_colors"`
	} `json:"style"`
}

type geoSpec struct {
	Geometry         string    `json:"geometry"`
	CellSizeMetres   *int      `json:"cell_size_m"`
	RadiusMetres     *int      `json:"radius_m"`
	CellSizePixels   *int      `json:"cell_size_px"`
	Thresholds       []float64 `json:"thresholds"`
	Layer            string    `json:"layer"`
	Radius           *int      `json:"radius_px"`
	CoordinateSystem struct {
		Type string `json:"type"`
		CRS  string `json:"crs"`
	} `json:"coordinate_system"`
	Encoding struct {
		X      string `json:"x"`
		Y      string `json:"y"`
		Color  string `json:"color"`
		Group  string `json:"group"`
		Weight string `json:"weight"`
		Shape  string `json:"shape"`
	} `json:"encoding"`
	View struct {
		Mode string `json:"mode"`
	} `json:"view"`
}

func parseArtifact(value any) (artifact, []byte, error) {
	var result artifact
	schema, err := resolveSchema()
	if err != nil {
		return result, nil, err
	}
	if err := schema.Validate(value); err != nil {
		return result, nil, err
	}
	data, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(data, &result)
	}
	return result, data, err
}

func (a artifact) kind() string {
	if a.Data.Type == "query_result" {
		return "result"
	}
	return "dataset"
}

func (a artifact) spec() (p Preview, sample bool, limit int, err error) {
	switch a.Type {
	case "table":
		var spec tableSpec
		err = json.Unmarshal(a.Visualization, &spec)
		p.Limit = spec.PageSize
		for _, column := range spec.Columns {
			p.Columns = append(p.Columns, column.Field)
		}
		limit = maxPage
	case "chart":
		var spec chartSpec
		err = json.Unmarshal(a.Visualization, &spec)
		p.Limit, limit = 2000, maxPoints
		if spec.ChartType == "pie" {
			p.Columns = distinct(spec.Encoding.Category, spec.Encoding.Value)
			p.Sort = []Sort{{Field: spec.Encoding.Category, Direction: "asc"}}
			break
		}
		p.Columns = distinct(spec.Encoding.X.Field, spec.Encoding.Y.Field, spec.Encoding.Series)
		sample = spec.ChartType == "scatter"
		if !sample {
			p.Sort = []Sort{{Field: spec.Encoding.X.Field, Direction: "asc"}}
		}
	case "geo":
		var spec geoSpec
		err = json.Unmarshal(a.Visualization, &spec)
		p.Columns = distinct(spec.Encoding.X, spec.Encoding.Y, spec.Encoding.Color, spec.Encoding.Weight, spec.Encoding.Shape, spec.Encoding.Group)
		p.Limit, limit, sample = maxPoints, maxPoints, true
	}
	return
}

func distinct(fields ...string) []string {
	result := []string{}
	seen := map[string]bool{"": true}
	for _, field := range fields {
		if !seen[field] {
			result = append(result, field)
			seen[field] = true
		}
	}
	return result
}

func classifyColumn(kind string) string {
	if strings.HasSuffix(kind, "[]") || strings.HasSuffix(kind, "]") {
		return "nominal"
	}
	switch kind {
	case "TINYINT", "SMALLINT", "INTEGER", "BIGINT", "HUGEINT",
		"UTINYINT", "USMALLINT", "UINTEGER", "UBIGINT", "UHUGEINT", "FLOAT", "DOUBLE", "BIGNUM":
		return "quantitative"
	case "DATE", "TIMESTAMP", "TIMESTAMP WITH TIME ZONE", "TIMESTAMP_NS", "TIMESTAMP_MS", "TIMESTAMP_S":
		return "temporal"
	}
	if strings.HasPrefix(kind, "DECIMAL(") {
		return "quantitative"
	}
	return "nominal"
}

func (a artifact) validateColumns(columns []Column) string {
	types := map[string]string{}
	for _, column := range columns {
		types[column.Name] = classifyColumn(column.Type)
	}
	p, _, _, _ := a.spec()
	for _, field := range p.Columns {
		if types[field] == "" {
			return "ANALYSIS_UNKNOWN_FIELD"
		}
	}
	switch a.Type {
	case "chart":
		var spec chartSpec
		_ = json.Unmarshal(a.Visualization, &spec)
		if spec.ChartType == "pie" {
			if types[spec.Encoding.Value] != "quantitative" || spec.Encoding.Value == spec.Encoding.Category {
				return "ANALYSIS_INVALID_ENCODING"
			}
			break
		}
		x, y := spec.Encoding.X, spec.Encoding.Y
		if y.Type != "quantitative" || types[y.Field] != y.Type ||
			x.Type != "nominal" && types[x.Field] != x.Type ||
			(spec.ChartType == "scatter" || spec.ChartType == "histogram") && x.Type != "quantitative" {
			return "ANALYSIS_INVALID_ENCODING"
		}
		if len(spec.Style.CategoryColors) > 0 &&
			(spec.ChartType != "bar" || x.Type != "nominal" || spec.Encoding.Series != "" || len(spec.Style.SeriesColors) > 0) {
			return "ANALYSIS_INVALID_ENCODING"
		}
	case "geo":
		var spec geoSpec
		_ = json.Unmarshal(a.Visualization, &spec)
		cs := spec.CoordinateSystem
		encoding := spec.Encoding
		geographic := cs.Type == "geographic" && cs.CRS == "EPSG:4326" && spec.View.Mode == "map"
		aggregate := spec.Layer != "" && spec.Layer != "points"
		if spec.Radius != nil && spec.Layer != "heatmap" ||
			spec.CellSizeMetres != nil && spec.Layer != "grid" && spec.Layer != "contour" ||
			spec.RadiusMetres != nil && spec.Layer != "hexagon" ||
			spec.CellSizePixels != nil && spec.Layer != "screen_grid" ||
			len(spec.Thresholds) > 0 && spec.Layer != "contour" {
			return "ANALYSIS_INVALID_ENCODING"
		}
		if spec.Geometry != "point" {
			for _, column := range columns {
				if column.Name == encoding.Shape && (column.Type == "VARCHAR" || column.Type == "JSON") &&
					geographic && spec.Layer == "" &&
					encoding.X == "" && encoding.Y == "" && encoding.Color == "" && encoding.Weight == "" && encoding.Group == "" {
					return ""
				}
			}
			return "ANALYSIS_INVALID_ENCODING"
		}
		if encoding.Shape != "" ||
			aggregate && (!geographic || encoding.Color != "" || spec.Layer != "heatmap" && encoding.Group != "" ||
				encoding.Weight != "" && types[encoding.Weight] != "quantitative") ||
			!aggregate && encoding.Weight != "" ||
			types[encoding.X] != "quantitative" || types[encoding.Y] != "quantitative" ||
			cs.Type == "geographic" && !geographic ||
			cs.Type == "projected" && (cs.CRS == "" || cs.CRS == "EPSG:4326") ||
			cs.Type == "cartesian" && cs.CRS != "" ||
			cs.Type != "geographic" && spec.View.Mode != "cartesian" {
			return "ANALYSIS_INVALID_ENCODING"
		}
	}
	return ""
}

func (s *Store) createArtifact(w http.ResponseWriter, r *http.Request) {
	var value any
	if !shared.Decode(w, r, &value, maxArtifact) {
		return
	}
	spec, data, err := parseArtifact(value)
	if err != nil || len(data) > maxArtifact {
		message := "Artifact exceeds the size limit."
		if err != nil {
			message = err.Error()
		}
		shared.JSON(w, 422, map[string]string{"code": "INVALID_ANALYSIS_ARTIFACT", "message": message})
		return
	}
	root, err := s.open()
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	defer root.Close()
	source, err := readDataset(root, spec.Data.Reference, spec.kind())
	if err != nil {
		shared.Error(w, 404, "ANALYSIS_DATA_UNAVAILABLE")
		return
	}
	if code := spec.validateColumns(source.Columns); code != "" {
		shared.Error(w, 422, code)
		return
	}
	s.artifacts.Lock()
	defer s.artifacts.Unlock()
	name := artifactName(spec.ID)
	var existing json.RawMessage
	if err := readNamedJSON(root, name, maxArtifact, &existing); err == nil {
		if !bytes.Equal(existing, data) {
			shared.Error(w, 409, "ANALYSIS_ARTIFACT_EXISTS")
			return
		}
		shared.JSON(w, 200, existing)
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	dir, err := root.Open(".")
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	names, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	count := 0
	for _, entry := range names {
		if strings.HasPrefix(entry, "artifact-") {
			count++
		}
	}
	if count >= 512 {
		shared.Error(w, 507, "ANALYSIS_STORAGE_LIMIT")
		return
	}
	tmp := ".artifact_" + shared.NewID("")
	defer root.Remove(tmp)
	if err := root.WriteFile(tmp, data, 0600); err != nil || root.Rename(tmp, name) != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 201, json.RawMessage(data))
}

func artifactName(id string) string {
	return "artifact-" + id + ".json"
}

func (s *Store) loadArtifact(w http.ResponseWriter, r *http.Request) (artifact, []byte, bool) {
	id := r.PathValue("artifact")
	if !shared.ValidID(id) {
		shared.Error(w, 404, "ANALYSIS_ARTIFACT_UNAVAILABLE")
		return artifact{}, nil, false
	}
	root, err := s.open()
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return artifact{}, nil, false
	}
	defer root.Close()
	var value any
	if readNamedJSON(root, artifactName(id), maxArtifact, &value) != nil {
		shared.Error(w, 404, "ANALYSIS_ARTIFACT_UNAVAILABLE")
		return artifact{}, nil, false
	}
	spec, data, err := parseArtifact(value)
	if err != nil || spec.ID != id {
		shared.Error(w, 422, "INVALID_ANALYSIS_ARTIFACT")
		return artifact{}, nil, false
	}
	return spec, data, true
}

func (s *Store) getArtifact(w http.ResponseWriter, r *http.Request) {
	if _, data, ok := s.loadArtifact(w, r); ok {
		shared.JSON(w, 200, json.RawMessage(data))
	}
}

func (s *Store) artifactData(w http.ResponseWriter, r *http.Request) {
	spec, _, ok := s.loadArtifact(w, r)
	if !ok {
		return
	}
	var input Preview
	if !shared.Decode(w, r, &input, shared.AnalysisRequestLimit) {
		return
	}
	p, sample, limit, err := spec.spec()
	if err != nil {
		shared.Error(w, 422, "INVALID_ANALYSIS_ARTIFACT")
		return
	}
	if spec.Type == "table" {
		var table tableSpec
		_ = json.Unmarshal(spec.Visualization, &table)
		if len(input.Columns) > 0 || len(input.Sort) > 0 && !table.Sortable || len(input.Filters) > 0 && !table.Filterable {
			shared.Error(w, 400, "INVALID_ANALYSIS_PREVIEW")
			return
		}
		if input.Limit != 0 {
			p.Limit = input.Limit
		}
		p.Cursor, p.Sort, p.Filters = input.Cursor, input.Sort, input.Filters
	} else if input.Limit != 0 || input.Cursor != "" || len(input.Columns)+len(input.Sort)+len(input.Filters) != 0 {
		shared.Error(w, 400, "INVALID_ANALYSIS_PREVIEW")
		return
	}
	s.serveData(w, r, spec.Data.Reference, spec.kind(), p, limit, sample)
}
