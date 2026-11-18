package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent-workbench/internal/shared"
)

func TestCodedErrorResponses(t *testing.T) {
	cases := []struct {
		name     string
		write    func(http.ResponseWriter, error)
		err      error
		status   int
		body     string
		bodyOnly bool
	}{
		{
			name: "wrapped runtime error", write: apiError,
			err:    fmt.Errorf("starting workspace: %w", shared.CodedError{Status: http.StatusTooManyRequests, Code: "QUOTA_EXCEEDED"}),
			status: http.StatusTooManyRequests,
			body:   "{\"code\":\"QUOTA_EXCEEDED\",\"message\":\"quota exceeded\",\"retryable\":false}\n",
		},
		{
			name: "uncoded runtime error", write: apiError,
			err:    errors.New("QUOTA_EXCEEDED"),
			status: http.StatusServiceUnavailable,
			body:   "{\"code\":\"RUNTIME_UNAVAILABLE\",\"message\":\"runtime unavailable\",\"retryable\":true}\n",
		},
		{
			name: "internal runtime code", write: apiError,
			err:    shared.CodedError{Code: "STOP_UNCONFIRMED"},
			status: http.StatusServiceUnavailable,
			body:   "{\"code\":\"RUNTIME_UNAVAILABLE\",\"message\":\"runtime unavailable\",\"retryable\":true}\n",
		},
		{
			name: "create body only", write: writeRequestErrorBody,
			err:      &shared.CodedError{Status: http.StatusConflict, Code: "WORKSPACE_BUSY"},
			status:   http.StatusConflict,
			body:     "{\"code\":\"WORKSPACE_BUSY\",\"message\":\"workspace busy\",\"retryable\":false}\n",
			bodyOnly: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			w.Header().Set("Retry-After", "30")
			tc.write(w, tc.err)
			if w.Code != tc.status || w.Body.String() != tc.body {
				t.Fatalf("got %d %q, want %d %q", w.Code, w.Body.String(), tc.status, tc.body)
			}
			contentType, cacheControl := "application/json", "no-store"
			if tc.bodyOnly {
				contentType, cacheControl = "", ""
			}
			h := w.Result().Header
			if h.Get("Retry-After") != "30" || h.Get("Content-Type") != contentType || h.Get("Cache-Control") != cacheControl {
				t.Fatalf("unexpected headers: %v", h)
			}
		})
	}
}
