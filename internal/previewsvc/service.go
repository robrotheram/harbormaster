// Package previewsvc promotes archived galleries into separately allocated,
// project-owned workspaces. The caller supplies an admitted, persisted allocation
// through the normal workspace service; this package never selects a backend.
package previewsvc

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"net/http"

	"fmt"
)

var ErrConflict = &shared.CodedError{Status: http.StatusConflict, Code: "IDEMPOTENCY_CONFLICT"}

type Runtime interface {
	Agent(context.Context, string, string, string, []byte) (int, []byte, error)
}
type Lifecycle interface {
	Start(context.Context, *shared.Workspace) error
	Stop(context.Context, *shared.Workspace) error
}
type Service struct {
	Runtime   Runtime
	Lifecycle Lifecycle
}

// Retain starts the separately registered allocation through the existing quota
// and lifecycle service and copies an immutable published artifact into it.
// Callers supply authorized workspace records and hold the coordination locks.
// On uncertain publication the allocation stays reserved for reconciliation.
func (s Service) Retain(ctx context.Context, coding shared.Workspace, retained *shared.Workspace, artifact string, ttl int, key string) ([]byte, error) {
	if !shared.ValidID(artifact) || ttl < 1 || ttl > 86400 || retained == nil || coding.ID == retained.ID || coding.ProjectID == "" || coding.ProjectID != retained.ProjectID || retained.HomeID == "" || retained.HomeID == coding.HomeID {
		return nil, fmt.Errorf("dedicated project allocation required")
	}
	if s.Lifecycle == nil {
		return nil, fmt.Errorf("retained lifecycle unavailable")
	}
	if err := s.Lifecycle.Start(ctx, retained); err != nil {
		return nil, err
	}
	if retained.State != "READY" {
		return nil, fmt.Errorf("dedicated running project allocation required")
	}
	if !shared.ValidKey(key) {
		return nil, fmt.Errorf("invalid receipt key")
	}

	source := coding.ProjectID + ":" + coding.ID + ":" + artifact
	receipt, _ := json.Marshal(map[string]any{"idempotency_key": key, "source_ref": source, "ttl_seconds": ttl})
	recovered, prior, receiptErr := s.Runtime.Agent(ctx, retained.ID, "POST", "/v1/services/receipt", receipt)
	if receiptErr != nil {
		return nil, receiptErr
	}
	if recovered == 200 {
		return prior, nil
	}
	if recovered == 409 {
		return nil, ErrConflict
	}
	if recovered != 404 {
		return nil, fmt.Errorf("receipt state unknown: %d", recovered)
	}
	status, snapshot, err := s.Runtime.Agent(ctx, coding.ID, "GET", "/v1/artifacts/"+artifact+"/content", nil)
	if err != nil || status != 200 {
		return nil, fmt.Errorf("published snapshot unavailable: %d %w", status, err)
	}
	if len(snapshot) > 16<<20 {
		return nil, fmt.Errorf("invalid snapshot budget")
	}
	payload, err := json.Marshal(map[string]any{"snapshot": snapshot, "ttl_seconds": ttl, "idempotency_key": key, "source_ref": source})
	if err != nil {
		return nil, err
	}
	status, body, err := s.Runtime.Agent(ctx, retained.ID, "POST", "/v1/services", payload)
	if err == nil && status == 409 {
		return nil, ErrConflict
	}
	if err != nil || status != 201 {
		return nil, fmt.Errorf("preview promotion failed: %d %w", status, err)
	}
	return body, nil
}

// ReleaseExpired asks the trusted lifecycle to stop only after guest revocation.
// A failed stop remains a reserved allocation under normal workspace policy.
func (s Service) ReleaseExpired(ctx context.Context, retained *shared.Workspace, id string) error {
	if retained == nil || !shared.ValidID(id) || s.Lifecycle == nil {
		return fmt.Errorf("invalid retained allocation")
	}
	status, body, err := s.Runtime.Agent(ctx, retained.ID, "GET", "/v1/services/"+id, nil)
	if err != nil || status != 200 {
		return fmt.Errorf("service state unknown")
	}
	var service struct {
		State string `json:"state"`
	}
	if json.Unmarshal(body, &service) != nil || (service.State != "expired" && service.State != "revoked") {
		return fmt.Errorf("service still active or unknown")
	}
	return s.Lifecycle.Stop(ctx, retained)
}
