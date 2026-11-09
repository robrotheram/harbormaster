package api

import "log/slog"

func (s *Server) reportWrite(stage, id string, err error) {
	if err != nil {
		slog.Error("metadata write failed", "stage", stage, "resource_id", id, "code", "STORAGE_UNAVAILABLE")
	}
}
