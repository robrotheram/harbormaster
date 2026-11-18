package shared

import (
	"errors"
	"net/http"
)

// These statuses let policy packages describe errors without importing the HTTP transport.
const (
	StatusBadRequest         = http.StatusBadRequest
	StatusConflict           = http.StatusConflict
	StatusTooManyRequests    = http.StatusTooManyRequests
	StatusServiceUnavailable = http.StatusServiceUnavailable
)

// CodedError carries a machine-readable code. A zero Status marks a code
// that needs an explicit HTTP mapping before it can be sent to a client.
type CodedError struct {
	Status int
	Code   string
}

func (e CodedError) Error() string { return e.Code }

func (e CodedError) details() CodedError { return e }

// ErrorDetails extracts a coded error, including wrapped values and pointers.
func ErrorDetails(err error) (CodedError, bool) {
	var coded interface{ details() CodedError }
	if errors.As(err, &coded) {
		return coded.details(), true
	}
	return CodedError{}, false
}

func ResolveHTTPError(err error, fallback CodedError) CodedError {
	if coded, ok := ErrorDetails(err); ok && coded.Status != 0 {
		return coded
	}
	return fallback
}

func WriteHTTPError(w http.ResponseWriter, err error, fallback CodedError) {
	coded := ResolveHTTPError(err, fallback)
	Error(w, coded.Status, coded.Code)
}

var ErrQuotaExceeded = CodedError{Status: http.StatusTooManyRequests, Code: "QUOTA_EXCEEDED"}
