// Package coordination implements explicit, bilateral exports. Bodies are kept
// behind project-owned Blob stores; SQL contains references and provenance only.
package coordination

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var ErrDenied = errors.New("exchange denied")
var ErrConflict = errors.New("publication conflict")
var ErrInvalid = errors.New("invalid coordination request")

// Schema is deliberately unnumbered; the application migration owner installs it.
const Schema = `CREATE TABLE IF NOT EXISTS project_exchanges (
 id TEXT PRIMARY KEY, source TEXT NOT NULL, destination TEXT NOT NULL,
 operation TEXT NOT NULL, source_approved INTEGER NOT NULL DEFAULT 0,
 destination_approved INTEGER NOT NULL DEFAULT 0, revoked INTEGER NOT NULL DEFAULT 0);
 CREATE TABLE IF NOT EXISTS exchange_exports (
 id TEXT PRIMARY KEY, exchange_id TEXT NOT NULL, source TEXT NOT NULL,
 destination TEXT NOT NULL, operation TEXT NOT NULL, reference TEXT NOT NULL,
 digest TEXT NOT NULL, actor TEXT NOT NULL, occurrence TEXT NOT NULL,
 UNIQUE(exchange_id, occurrence));`

type Grant struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Operation   string `json:"operation"`
}
type Export struct {
	ID          string `json:"id"`
	ExchangeID  string `json:"exchange_id"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Operation   string `json:"operation"`
	Reference   string `json:"reference"`
	Digest      string `json:"digest"`
	Actor       string `json:"actor"`
	Occurrence  string `json:"occurrence"`
}

// BlobStore must be bound to one project by the host. Put is immutable and
// idempotent. Get must not interpret references as paths outside that project.
type BlobStore interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
}
type Service struct {
	DB    *sql.DB
	Blobs func(project string) (BlobStore, error)
	mu    sync.Mutex
}

func valid(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-", c) {
			return false
		}
	}
	return true
}
func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (s *Service) Create(ctx context.Context, actor string, g Grant) error {
	if actor != g.Source || !valid(g.ID) || !valid(g.Source) || !valid(g.Destination) || g.Source == g.Destination || (g.Operation != "summary" && g.Operation != "result" && g.Operation != "work-request") {
		return ErrDenied
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO project_exchanges(id,source,destination,operation,source_approved) VALUES($1,$2,$3,$4,1)`, g.ID, g.Source, g.Destination, g.Operation)
	return err
}

// Approve requires independent authentication as the approving endpoint. A
// revoked grant is terminal: a fresh human-configured grant is required.
func (s *Service) Approve(ctx context.Context, actor, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.DB.ExecContext(ctx, `UPDATE project_exchanges SET destination_approved=1 WHERE id=$1 AND destination=$2 AND revoked=0`, id, actor)
	return affected(r, e)
}
func affected(r sql.Result, e error) error {
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return ErrDenied
	}
	return nil
}
func (s *Service) Revoke(ctx context.Context, actor, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.DB.ExecContext(ctx, `UPDATE project_exchanges SET revoked=1 WHERE id=$1 AND (source=$2 OR destination=$2)`, id, actor)
	return affected(r, e)
}
func (s *Service) grant(ctx context.Context, id string) (Grant, error) {
	var g Grant
	e := s.DB.QueryRowContext(ctx, `SELECT id,source,destination,operation FROM project_exchanges WHERE id=$1 AND source_approved=1 AND destination_approved=1 AND revoked=0`, id).Scan(&g.ID, &g.Source, &g.Destination, &g.Operation)
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrDenied
	}
	return g, e
}

// Publish accepts only an explicitly selected payload, never reads histories,
// memories, or arbitrary project paths. Occurrence is the factory occurrence ID.
func (s *Service) Publish(ctx context.Context, actor, id, occurrence string, body []byte) (Export, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var x Export
	g, e := s.grant(ctx, id)
	if e != nil {
		return x, e
	}
	if actor != g.Source {
		return x, ErrDenied
	}
	if !valid(occurrence) || len(body) == 0 || len(body) > 65536 {
		return x, ErrInvalid
	}
	key := Digest([]byte(id + ":" + occurrence))
	x = Export{key, id, g.Source, g.Destination, g.Operation, key, Digest(body), actor, occurrence}
	old, e := s.export(ctx, key)
	if e == nil {
		if old.Digest != x.Digest {
			return x, ErrConflict
		}
		return old, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return x, e
	}
	blobs, e := s.Blobs(g.Source)
	if e != nil {
		return x, e
	}
	if e = blobs.Put(ctx, key, body); e != nil {
		return x, e
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO exchange_exports(id,exchange_id,source,destination,operation,reference,digest,actor,occurrence) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, x.ID, x.ExchangeID, x.Source, x.Destination, x.Operation, x.Reference, x.Digest, x.Actor, x.Occurrence)
	return x, e
}
func (s *Service) export(ctx context.Context, id string) (x Export, e error) {
	e = s.DB.QueryRowContext(ctx, `SELECT id,exchange_id,source,destination,operation,reference,digest,actor,occurrence FROM exchange_exports WHERE id=$1`, id).Scan(&x.ID, &x.ExchangeID, &x.Source, &x.Destination, &x.Operation, &x.Reference, &x.Digest, &x.Actor, &x.Occurrence)
	return
}
func (s *Service) Read(ctx context.Context, actor, id string) (Export, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(ctx, actor, id)
}
func (s *Service) read(ctx context.Context, actor, id string) (Export, []byte, error) {
	x, e := s.export(ctx, id)
	if e != nil {
		return Export{}, nil, ErrDenied
	}
	g, e := s.grant(ctx, x.ExchangeID)
	if e != nil {
		return Export{}, nil, e
	}
	if actor != g.Destination || x.Source != g.Source || x.Destination != g.Destination || x.Operation != g.Operation {
		return Export{}, nil, ErrDenied
	}
	b, e := s.Blobs(x.Source)
	if e != nil {
		return x, nil, e
	}
	body, e := b.Get(ctx, x.Reference)
	if e == nil && Digest(body) != x.Digest {
		e = fmt.Errorf("export digest mismatch")
	}
	return x, body, e
}

// Consume serializes revocation against work submission for the single control-plane owner.
func (s *Service) Consume(ctx context.Context, actor, id string, submit func([]byte, string) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, body, e := s.read(ctx, actor, id)
	if e != nil {
		return e
	}
	if x.Operation != "work-request" {
		return ErrDenied
	}
	return submit(body, "exchange-"+x.ID)
}
