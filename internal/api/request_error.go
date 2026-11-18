package api

import (
	"encoding/json"
	"net/http"

	"agent-workbench/internal/shared"
)

var storageUnavailable = shared.CodedError{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE"}

func writeRequestError(w http.ResponseWriter, err error) {
	shared.WriteHTTPError(w, err, storageUnavailable)
}

// Some create routes historically forwarded only the status and error body.
func writeRequestErrorBody(w http.ResponseWriter, err error) {
	response := shared.ResolveHTTPError(err, storageUnavailable)
	w.WriteHeader(response.Status)
	_ = json.NewEncoder(w).Encode(shared.NewAPIError(response.Status, response.Code))
}

func createStatus(created bool) int {
	if created {
		return http.StatusCreated
	}
	return http.StatusOK
}
