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

// ConsumptionSchema is installed by migration 6, never by runtime startup.
const ConsumptionSchema = `CREATE TABLE IF NOT EXISTS exchange_consumptions (
 export_id TEXT PRIMARY KEY, destination TEXT NOT NULL, work_id TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS exchange_inbox_destination ON exchange_exports(destination,id);`

type InboxPage struct {
	Items []Export `json:"items"`
	Next  string   `json:"next"`
}

// Inbox includes durable consumption receipts for recovery before work_run.
// It uses a keyset cursor over immutable IDs. Consumers must finish every
// page before restarting at the beginning; pending old requests cannot starve others.
func (s *Service) Inbox(ctx context.Context, actor, after string, limit int) (InboxPage, error) {
	page := InboxPage{Items: []Export{}}
	if !valid(actor) || (after != "" && !valid(after)) || limit < 1 || limit > 100 {
		return page, ErrInvalid
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT x.id,x.exchange_id,x.source,x.destination,x.operation,x.reference,x.digest,x.actor,x.occurrence,COALESCE(c.work_id,'')
 FROM exchange_exports x JOIN project_exchanges g ON g.id=x.exchange_id
 LEFT JOIN exchange_consumptions c ON c.export_id=x.id
 WHERE x.destination=$1 AND x.id>$2
 AND g.source_approved=1 AND g.destination_approved=1 AND g.revoked=0
 AND x.source=g.source AND x.destination=g.destination AND x.operation=g.operation AND x.actor=g.source
 ORDER BY x.id LIMIT $3`, actor, after, limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var x Export
		if err = rows.Scan(&x.ID, &x.ExchangeID, &x.Source, &x.Destination, &x.Operation, &x.Reference, &x.Digest, &x.Actor, &x.Occurrence, &x.WorkID); err != nil {
			return page, err
		}
		page.Items = append(page.Items, x)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.Next = page.Items[limit-1].ID
	}
	return page, nil
}

type Grant struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Operation   string `json:"operation"`
}
type Export struct {
	WorkID      string `json:"work_id,omitempty"`
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
	DB     *sql.DB
	Blobs  func(project string) (BlobStore, error)
	mu     sync.Mutex
	grants map[string]*exchangeLock
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
	unlock := s.lockGrant(id)
	defer unlock()
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
	unlock := s.lockGrant(id)
	defer unlock()
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
	unlock := s.lockGrant(id)
	defer unlock()
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
	x = Export{ID: key, ExchangeID: id, Source: g.Source, Destination: g.Destination, Operation: g.Operation, Reference: key, Digest: Digest(body), Actor: actor, Occurrence: occurrence}
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
	identity, err := s.export(ctx, id)
	if err != nil {
		return Export{}, nil, ErrDenied
	}
	unlock := s.lockGrant(identity.ExchangeID)
	defer unlock()
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
	if actor != g.Destination || x.Source != g.Source || x.Destination != g.Destination || x.Operation != g.Operation || x.Actor != g.Source {
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
func (s *Service) Consume(ctx context.Context, actor, id string, submit func([]byte, string) (string, error)) error {
	identity, err := s.export(ctx, id)
	if err != nil {
		return ErrDenied
	}
	unlock := s.lockGrant(identity.ExchangeID)
	defer unlock()
	x, body, e := s.read(ctx, actor, id)
	if e != nil {
		return e
	}
	if x.Operation != "work-request" {
		return ErrDenied
	}
	workID, err := submit(body, "exchange-"+x.ID)
	if err != nil {
		return err
	}
	if !valid(workID) {
		return ErrInvalid
	}
	// Work creation commits first. A lost receipt retries the same factory key.
	_, err = s.DB.ExecContext(ctx, `INSERT INTO exchange_consumptions(export_id,destination,work_id) VALUES($1,$2,$3) ON CONFLICT(export_id) DO NOTHING`, x.ID, actor, workID)
	return err
}

// Only operations on the same exchange serialize across private-store IO.
// The map mutex is never held during a network call and idle entries are freed.
type exchangeLock struct {
	mu         sync.Mutex
	references int
}

func (s *Service) lockGrant(id string) func() {
	s.mu.Lock()
	if s.grants == nil {
		s.grants = map[string]*exchangeLock{}
	}
	entry := s.grants[id]
	if entry == nil {
		entry = &exchangeLock{}
		s.grants[id] = entry
	}
	entry.references++
	s.mu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		s.mu.Lock()
		defer s.mu.Unlock()
		entry.references--
		if entry.references == 0 {
			delete(s.grants, id)
		}
	}
}
