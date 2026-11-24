package workspacetools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"agent-workbench/internal/shared"
)

func Analysis(ctx context.Context, args []string, out io.Writer) error {
	const usage = "usage: workbench-analysis list | schema ID | path ID | preview ID REQUEST.json | ingest FILE | register-result FILE | artifact ID | register-artifact SPEC.json"
	token := os.Getenv("WORKBENCH_WORKSPACE_GRANT")
	if len(args) < 1 || len(args) > 3 {
		return errors.New(usage)
	}
	if token == "" {
		return errors.New("run workbench-analysis inside an agent workspace")
	}
	var id string
	if len(args) >= 2 {
		id = args[1]
	}
	var endpoint string
	method := http.MethodGet
	var body *os.File
	var size int64
	var err error
	switch args[0] {
	case "list":
		if len(args) == 1 {
			endpoint = "/datasets"
		}
	case "schema", "path":
		if len(args) == 2 && shared.ValidID(id) {
			switch {
			case strings.HasPrefix(id, "dataset_"):
				endpoint = "/datasets/" + id + "/schema"
			case strings.HasPrefix(id, "result_"):
				endpoint = "/results/" + id + "/schema"
			}
		}
	case "preview":
		if len(args) == 3 && shared.ValidID(id) && strings.HasPrefix(id, "dataset_") {
			endpoint = "/datasets/" + id + "/preview"
			body, size, err = analysisInput(args[2], shared.AnalysisRequestLimit)
		}
	case "ingest", "register-result":
		if len(args) == 2 && shared.AnalysisFilename(filepath.Base(id)) {
			endpoint = "/datasets/upload"
			if args[0] == "register-result" {
				endpoint = "/results/upload"
			}
			endpoint += "?name=" + url.QueryEscape(filepath.Base(id))
			body, size, err = analysisInput(id, shared.AnalysisUploadLimit)
		}
	case "register-artifact":
		if len(args) == 2 {
			endpoint = "/artifacts"
			body, size, err = analysisInput(id, shared.AnalysisRequestLimit)
		}
	case "artifact":
		if len(args) == 2 && shared.ValidID(id) {
			endpoint = "/artifacts/" + id
		}
	}
	if endpoint == "" {
		return errors.New(usage)
	}
	if err != nil {
		return err
	}
	var input io.Reader
	if body != nil {
		defer body.Close()
		input = io.LimitReader(body, size)
		method = http.MethodPost
	}
	client := &http.Client{
		Timeout:       shared.AnalysisUploadTimeout,
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:8080/v1/analysis"+endpoint, input)
	if err != nil {
		return err
	}
	request.ContentLength = size
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
		if args[0] == "ingest" || args[0] == "register-result" {
			request.Header.Set("Content-Type", "application/octet-stream")
		}
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("workspace analysis service unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, shared.AnalysisResponseLimit+1))
	if err != nil || len(data) > shared.AnalysisResponseLimit || !json.Valid(data) {
		return errors.New("invalid response from workspace analysis service")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var result shared.APIError
		_ = json.Unmarshal(data, &result)
		return fmt.Errorf("analysis request failed (HTTP %d): %s: %s", response.StatusCode, result.Code, result.Message)
	}
	if args[0] == "path" {
		var result struct {
			ID     string `json:"dataset_id"`
			Format string `json:"format"`
		}
		if json.Unmarshal(data, &result) != nil || result.ID != id || (result.Format != "csv" && result.Format != "parquet") {
			return errors.New("invalid dataset metadata")
		}
		_, err = fmt.Fprintln(out, filepath.Join(shared.Env("DATA_DIR", "/data"), "analysis", id, "data."+result.Format))
		return err
	}
	_, err = out.Write(data)
	return err
}

func analysisInput(name string, limit int64) (*os.File, int64, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > limit {
		f.Close()
		return nil, 0, errors.New("input must be a nonempty regular file within the size limit")
	}
	return f, info.Size(), nil
}
