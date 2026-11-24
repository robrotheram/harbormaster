package shared

import (
	"net/http"
	"path"
	"strings"
	"time"
	"unicode"
)

const (
	AnalysisUploadLimit   = 1 << 30
	AnalysisRequestLimit  = 32 << 10
	AnalysisResponseLimit = 1 << 20
	AnalysisUploadTimeout = 10 * time.Minute
	AnalysisUploadPath    = "/v1/analysis/datasets/upload"
)

func AnalysisFilename(name string) bool {
	if name == "" || len(name) > 200 || name != path.Base(name) || strings.Contains(name, "\\") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".csv", ".parquet":
		return true
	}
	return false
}

// AnalysisAgentRoute is the subset of analysis routes exposed to the controller.
func AnalysisAgentRoute(method, route string) bool {
	if method == http.MethodPost && route == AnalysisUploadPath {
		return true
	}
	prefix, ok := strings.CutPrefix(route, "/v1/analysis/")
	if !ok {
		return false
	}
	parts := strings.Split(prefix, "/")
	switch parts[0] {
	case "datasets":
		return len(parts) == 1 && method == http.MethodGet ||
			len(parts) == 3 && ValidID(parts[1]) &&
				(parts[2] == "schema" && method == http.MethodGet || parts[2] == "preview" && method == http.MethodPost)
	case "artifacts":
		return len(parts) == 2 && ValidID(parts[1]) && method == http.MethodGet ||
			len(parts) == 3 && ValidID(parts[1]) && parts[2] == "data" && method == http.MethodPost
	}
	return false
}
