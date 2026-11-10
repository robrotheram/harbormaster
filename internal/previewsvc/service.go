// Package previewsvc promotes archived galleries into separately allocated,
// project-owned workspaces. The caller supplies an admitted, persisted allocation
// through the normal workspace service; this package never selects a backend.
package previewsvc

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrConflict = errors.New("IDEMPOTENCY_CONFLICT")

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
// and lifecycle service. Callers hold the normal workspace coordination locks.
func (s Service) Retain(ctx context.Context, coding shared.Workspace, retained *shared.Workspace, artifact string, ttl int, keys ...string) ([]byte, error) {
	if !shared.ValidID(artifact) || ttl < 1 || ttl > 86400 || retained == nil || coding.ID == retained.ID || coding.ProjectID == "" || coding.ProjectID != retained.ProjectID || retained.HomeID == "" || retained.HomeID == coding.HomeID {
		return nil, fmt.Errorf("dedicated project allocation required")
	}
	if s.Lifecycle == nil {
		return nil, fmt.Errorf("retained lifecycle unavailable")
	}
	if err := s.Lifecycle.Start(ctx, retained); err != nil {
		return nil, err
	}
	// On uncertain publication leave the allocation reserved for reconciliation.
	return s.Promote(ctx, coding, *retained, artifact, ttl, keys...)
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

// Promote copies an immutable published artifact, rather than mounting the coding
// checkout. Both workspace records must come from an authorized project lookup.
func (s Service) Promote(ctx context.Context, coding, retained shared.Workspace, artifact string, ttl int, keys ...string) ([]byte, error) {
	if coding.ID == retained.ID || coding.ProjectID == "" || coding.ProjectID != retained.ProjectID || retained.HomeID == coding.HomeID || retained.HomeID == "" || retained.State != "READY" || !shared.ValidID(artifact) {
		return nil, fmt.Errorf("dedicated running project allocation required")
	}
	key := "retain-" + coding.ID + "-" + artifact
	if len(keys) > 0 {
		key = keys[0]
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
	if len(snapshot) > 16<<20 || ttl < 1 || ttl > 86400 {
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

// Attachment fails closed until the caller implements an isolated preview host
// and session. Fetch is a byte transport and must never be copied onto UI origin.
func (s Service) Attach(context.Context, string) error {
	return fmt.Errorf("ISOLATED_PREVIEW_ATTACHMENT_UNSUPPORTED")
}
