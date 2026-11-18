package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"agent-workbench/internal/shared"
)

type requestError struct {
	status int
	code   string
}

func (e requestError) Error() string { return e.code }

func requestErrorDetails(err error) requestError {
	var response requestError
	if errors.As(err, &response) {
		return response
	}
	return requestError{http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE"}
}

func writeRequestError(w http.ResponseWriter, err error) {
	response := requestErrorDetails(err)
	shared.Error(w, response.status, response.code)
}

// Some create routes historically forwarded only the status and error body.
func writeRequestErrorBody(w http.ResponseWriter, err error) {
	response := requestErrorDetails(err)
	w.WriteHeader(response.status)
	_ = json.NewEncoder(w).Encode(shared.NewAPIError(response.status, response.code))
}

func createStatus(created bool) int {
	if created {
		return http.StatusCreated
	}
	return http.StatusOK
}
