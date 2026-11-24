package analysis

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestChartStylesAndPieEncoding(t *testing.T) {
	const envelope = `{"schema_version":"1.0","artifact_id":"observations","type":"chart","title":"Observations","data":{"source_type":"query_result","reference":"result_counts"},"visualization":`
	const nominalAxes = `"x":{"field":"fur","type":"nominal"},"y":{"field":"count","type":"quantitative"}`
	const pie = `"chart_type":"pie","encoding":{"category":"fur","value":"count"}`
	columns := []Column{{Name: "count", Type: "BIGINT"}, {Name: "fur", Type: "VARCHAR"}}
	tests := []struct {
		name, chart string
		invalidJSON bool
		columnError string
	}{
		{"existing bar", `"chart_type":"bar","encoding":{` + nominalAxes + `}`, false, ""},
		{"colored bar", `"chart_type":"bar","encoding":{` + nominalAxes + `},"style":{"category_colors":{"Black":"#68768c"},"gridlines":{"x":true,"y":false}}`, false, ""},
		{"colored line", `"chart_type":"line","encoding":{` + nominalAxes + `},"style":{"series_colors":{"count":"#3d79a8"}}`, false, ""},
		{"colored pie", pie + `,"style":{"category_colors":{"Black":"#68768c"}}`, false, ""},
		{"unknown field", `"chart_type":"pie","encoding":{"category":"fur","value":"missing"}`, false, "ANALYSIS_UNKNOWN_FIELD"},
		{"pie value must be numeric", `"chart_type":"pie","encoding":{"category":"count","value":"fur"}`, false, "ANALYSIS_INVALID_ENCODING"},
		{"pie fields must differ", `"chart_type":"pie","encoding":{"category":"count","value":"count"}`, false, "ANALYSIS_INVALID_ENCODING"},
		{"pie requires value", `"chart_type":"pie","encoding":{"category":"fur"}`, true, ""},
		{"pie rejects gridlines", pie + `,"style":{"gridlines":{"x":false}}`, true, ""},
		{"pie rejects extra encodings", `"chart_type":"pie","encoding":{"category":"fur","value":"count","series":"fur"}`, true, ""},
		{"color must be a hex value", pie + `,"style":{"category_colors":{"Black":"red"}}`, true, ""},
		{"renderer options rejected", pie + `,"style":{"tooltip":{"formatter":"{}"}}`, true, ""},
		{"category colors require a single bar series", `"chart_type":"bar","encoding":{` + nominalAxes + `,"series":"fur"},"style":{"category_colors":{"Black":"#68768c"}}`, false, "ANALYSIS_INVALID_ENCODING"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var value any
			if err := json.Unmarshal([]byte(envelope+"{"+tt.chart+"}}"), &value); err != nil {
				t.Fatal(err)
			}
			spec, _, err := parseArtifact(value)
			if tt.invalidJSON {
				if err == nil {
					t.Fatal("accepted invalid chart JSON")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if code := spec.validateColumns(columns); code != tt.columnError {
				t.Fatalf("column validation = %q, want %q", code, tt.columnError)
			}
			if tt.name == "colored pie" {
				p, sample, limit, err := spec.spec()
				if err != nil || sample || p.Limit != 2000 || limit != maxPoints ||
					!reflect.DeepEqual(p.Columns, []string{"fur", "count"}) ||
					!reflect.DeepEqual(p.Sort, []Sort{{Field: "fur", Direction: "asc"}}) {
					t.Fatalf("pie projection = %+v, sample %v, limit %d, err %v", p, sample, limit, err)
				}
			}
		})
	}
}
