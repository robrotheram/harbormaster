package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var invalid = errors.New("INVALID_REQUEST")

type controlClient struct {
	base, token string
	client      *http.Client
}
type accessContextKey struct{}
type projectAccess struct{ token, project string }

// Only fixed route builders call do. A retry uses identical bytes and the
// caller's original idempotency key, including after ambiguous network errors.
func (c *controlClient) do(ctx context.Context, method, path string, input, output any) error {
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return invalid
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.New("RUNTIME_UNAVAILABLE")
			case <-timer.C:
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
		if err != nil {
			return errors.New("RUNTIME_UNAVAILABLE")
		}
		token := c.token
		if access, ok := ctx.Value(accessContextKey{}).(projectAccess); ok {
			token = access.token
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if input != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.client.Do(req)
		if err != nil {
			if attempt == 0 && ctx.Err() == nil {
				continue
			}
			return errors.New("RUNTIME_UNAVAILABLE")
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
		resp.Body.Close()
		if resp.StatusCode == 503 && attempt == 0 && ctx.Err() == nil {
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			var e struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(data, &e)
			switch e.Code {
			case "EXCHANGE_DENIED", "PUBLICATION_CONFLICT", "COORDINATION_UNAVAILABLE", "INVALID_REQUEST", "UNAUTHORIZED", "NOT_FOUND_OR_FORBIDDEN", "WORKSPACE_BUSY", "IDEMPOTENCY_CONFLICT", "QUOTA_EXCEEDED", "NOT_READY", "RUNTIME_UNAVAILABLE", "AGENT_PROTOCOL_UNAVAILABLE":
				return errors.New(e.Code)
			}
			return errors.New("RUNTIME_UNAVAILABLE")
		}
		if readErr != nil || len(data) > 2<<20 || json.Unmarshal(data, output) != nil {
			return errors.New("AGENT_PROTOCOL_UNAVAILABLE")
		}
		return nil
	}
	return errors.New("RUNTIME_UNAVAILABLE")
}
