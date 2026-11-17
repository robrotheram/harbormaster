package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"unicode/utf8"

	"agent-workbench/internal/shared"
)

type reviewScreenshot struct {
	Content []byte `json:"content"`
	Note    string `json:"note"`
}

func validReviewScreenshots(shots []reviewScreenshot) bool {
	if len(shots) > 4 {
		return false
	}
	for _, shot := range shots {
		if len(shot.Content) == 0 || len(shot.Content) > shared.HandoffLimit || len(shot.Note) > 2000 || !utf8.ValidString(shot.Note) {
			return false
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(shot.Content))
		if err != nil || (format != "png" && format != "jpeg") || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16000000 {
			return false
		}
	}
	return true
}

// Publish immutable private files before dispatch. Retries verify existing bytes.
func (s *Server) publishReviewScreenshots(w http.ResponseWriter, r *http.Request, workspace, key string, shots []reviewScreenshot) (string, bool) {
	context := ""
	for i, shot := range shots {
		_, format, _ := image.DecodeConfig(bytes.NewReader(shot.Content))
		path := fmt.Sprintf("work-requests/%s-screenshot-%d.%s", key, i+1, format)
		body, _ := json.Marshal(shared.FileHandoff{Path: path, Content: shot.Content})
		status, reply, err := s.Runtime.Agent(r.Context(), workspace, "POST", "/v1/files/write-new", body)
		if err == nil && status == 409 {
			var failure shared.APIError
			_ = json.Unmarshal(reply, &failure)
			if failure.Code == "DESTINATION_EXISTS" {
				body, _ = json.Marshal(shared.FileHandoff{Path: path})
				status, reply, err = s.Runtime.Agent(r.Context(), workspace, "POST", "/v1/files/read", body)
				var prior shared.FileHandoff
				if err == nil && status == 200 && json.Unmarshal(reply, &prior) == nil && sha256.Sum256(prior.Content) == sha256.Sum256(shot.Content) {
					status = 201
				}
			}
		}
		if err != nil || status != 201 {
			shared.Error(w, 503, "SCREENSHOT_PUBLICATION_FAILED")
			return "", false
		}
		context += fmt.Sprintf("\n\nReview screenshot %d: %s\nNote: %s", i+1, path, shot.Note)
	}
	return context, true
}
