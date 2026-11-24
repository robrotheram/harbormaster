package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"agent-workbench/internal/shared"
)

type datasetUploader interface {
	UploadDataset(context.Context, string, string, io.Reader) (int, []byte, error)
}

func (s *Server) registerAnalysis(mux *http.ServeMux) {
	const base = "/v1/workspaces/{id}/analysis/"
	mux.HandleFunc("GET "+base+"datasets", s.analysis)
	mux.HandleFunc("POST "+base+"datasets/upload", s.analysis)
	mux.HandleFunc("GET "+base+"datasets/{dataset}/schema", s.analysis)
	mux.HandleFunc("POST "+base+"datasets/{dataset}/preview", s.analysis)
	mux.HandleFunc("GET "+base+"artifacts/{artifact}", s.analysis)
	mux.HandleFunc("GET "+base+"artifacts", s.analysis)
	mux.HandleFunc("POST "+base+"artifacts/{artifact}/data", s.analysis)
	mux.HandleFunc("POST "+base+"artifacts/{artifact}/delete", s.analysis)
	const saved = "/v1/projects/{id}/analysis/saved-visualizations"
	mux.HandleFunc("GET "+saved, s.savedVisualizations)
	mux.HandleFunc("POST "+saved, s.savedVisualizations)
	const details = "/v1/projects/{id}/analysis/visualization-details"
	mux.HandleFunc("GET "+details, s.visualizationDetails)
	mux.HandleFunc("POST "+details, s.visualizationDetails)
}

func (s *Server) analysis(w http.ResponseWriter, r *http.Request) {
	ws, ok := s.workspace(w, r)
	if !ok {
		return
	}
	p, _ := r.Context().Value(principalKey{}).(principal)
	if strings.HasSuffix(r.URL.Path, "/delete") && !p.admin && p.human.ID == "" {
		shared.Error(w, 403, "FORBIDDEN")
		return
	}
	if ws.Shared && !p.admin {
		shared.Error(w, 409, "PROJECT_ISOLATED_WORKERS_REQUIRED")
		return
	}
	if project := r.URL.Query().Get("project_id"); project != "" && project != ws.ProjectID {
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return
	}
	suffix, _ := strings.CutPrefix(r.URL.Path, "/v1/workspaces/"+ws.ID+"/analysis/")
	route := "/v1/analysis/" + suffix
	if !shared.AnalysisAgentRoute(r.Method, route) {
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return
	}
	if route == shared.AnalysisUploadPath {
		s.uploadDataset(w, r, ws.ID)
		return
	}
	var body []byte
	if r.Method == http.MethodPost {
		var err error
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, shared.AnalysisRequestLimit))
		if err != nil || !json.Valid(body) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
	}
	s.factoryForward(w, r, ws.ID, route, body)
}

func (s *Server) uploadDataset(w http.ResponseWriter, r *http.Request, worker string) {
	name := r.URL.Query()["name"]
	if len(name) != 1 || !shared.AnalysisFilename(name[0]) || r.Header.Get("Content-Type") != "application/octet-stream" {
		shared.Error(w, 400, "INVALID_DATASET_UPLOAD")
		return
	}
	if r.ContentLength > shared.AnalysisUploadLimit {
		shared.Error(w, 413, "DATASET_TOO_LARGE")
		return
	}
	uploader, ok := s.Runtime.(datasetUploader)
	if !ok {
		shared.Error(w, 503, "DATASET_UPLOAD_UNAVAILABLE")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), shared.AnalysisUploadTimeout)
	defer cancel()
	route := shared.AnalysisUploadPath + "?name=" + url.QueryEscape(name[0])
	status, data, err := uploader.UploadDataset(ctx, worker, route, http.MaxBytesReader(w, r.Body, shared.AnalysisUploadLimit))
	if err != nil || status < 200 || status > 599 || !json.Valid(data) {
		shared.Error(w, 503, "DATASET_UPLOAD_UNAVAILABLE")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
