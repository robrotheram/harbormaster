package api

import (
	"net/http"

	"agent-workbench/internal/shared"
)

func decodeRouteBody(w http.ResponseWriter, r *http.Request, value any) bool {
	return shared.Decode(w, r, value, shared.APIBodyLimit(r.Pattern))
}
