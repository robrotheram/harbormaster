package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"agent-workbench/internal/shared"
	"github.com/jackc/pgx/v5/stdlib"
)

func (s *Store) migrateHistoryCache(ctx context.Context, tx *sql.Tx) error {
	_, postgres := s.DB.Driver().(*stdlib.Driver)
	identity := "INTEGER PRIMARY KEY"
	if postgres {
		identity = "BIGSERIAL PRIMARY KEY"
	}
	for _, statement := range []string{
		`CREATE TABLE history_cache_state(workspace_id TEXT PRIMARY KEY REFERENCES workspaces(id), cursor BIGINT NOT NULL, source_high BIGINT NOT NULL, bytes_used BIGINT NOT NULL, capture_limited BOOLEAN NOT NULL, synced_at TEXT NOT NULL)`,
		`CREATE TABLE history_cache(audit_seq ` + identity + `, project_id TEXT NOT NULL, workspace_id TEXT NOT NULL REFERENCES workspaces(id), seq BIGINT NOT NULL, episode_id TEXT NOT NULL, kind TEXT NOT NULL, body TEXT NOT NULL, UNIQUE(workspace_id,seq))`,
		`CREATE INDEX history_cache_project_audit ON history_cache(project_id,audit_seq)`,
		`CREATE INDEX history_cache_episode ON history_cache(workspace_id,episode_id,seq)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

type HistoryCacheState struct {
	Cursor         int64  `json:"cached_through"`
	SourceHigh     int64  `json:"source_high_watermark"`
	BytesUsed      int64  `json:"-"`
	CaptureLimited bool   `json:"capture_limited"`
	SyncedAt       string `json:"synced_at"`
}

func (s *Store) HistoryCacheState(ctx context.Context, workspace string) (HistoryCacheState, error) {
	var state HistoryCacheState
	err := s.DB.QueryRowContext(ctx, `SELECT cursor,source_high,bytes_used,capture_limited,synced_at FROM history_cache_state WHERE workspace_id=$1`, workspace).
		Scan(&state.Cursor, &state.SourceHigh, &state.BytesUsed, &state.CaptureLimited, &state.SyncedAt)
	return state, err
}

// AppendHistoryCache advances a worker cursor in the same transaction as its
// records. If two reconcilers read the same page, only one can advance it.
func (s *Store) AppendHistoryCache(ctx context.Context, ws shared.Workspace, after int64, page shared.HistoryPage, at time.Time) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	cursor := after
	if n := len(page.Records); n > 0 {
		cursor = page.Records[n-1].Seq
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO history_cache_state(workspace_id,cursor,source_high,bytes_used,capture_limited,synced_at)
		VALUES($1,0,0,0,FALSE,'') ON CONFLICT(workspace_id) DO NOTHING`, ws.ID); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE history_cache_state SET cursor=$1,
		source_high=CASE WHEN source_high>$2 THEN source_high ELSE $2 END,
		bytes_used=CASE WHEN bytes_used>$3 THEN bytes_used ELSE $3 END,
		capture_limited=capture_limited OR $4,synced_at=$5 WHERE workspace_id=$6 AND cursor=$7`,
		cursor, page.HighWatermark, page.BytesUsed, page.CaptureLimited, at.UTC().Format(time.RFC3339Nano), ws.ID, after)
	if err != nil {
		return false, err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return false, err
	}
	for _, record := range page.Records {
		record.Purpose = ""
		body, err := json.Marshal(record)
		if err != nil {
			return false, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO history_cache(project_id,workspace_id,seq,episode_id,kind,body) VALUES($1,$2,$3,$4,$5,$6)`,
			ws.ProjectID, ws.ID, record.Seq, record.EpisodeID, record.Kind, string(body)); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

type HistoryFilter struct {
	After, Before, Until int64
	Episode              string
	Descending           bool
}

func (s *Store) CachedHistory(ctx context.Context, workspace string, filter HistoryFilter) (shared.HistoryPage, HistoryCacheState, error) {
	page := shared.HistoryPage{SchemaVersion: 1, Records: []shared.HistoryRecord{}}
	state, err := s.HistoryCacheState(ctx, workspace)
	if err != nil {
		return page, state, err
	}
	page.HighWatermark, page.BytesUsed, page.CaptureLimited = state.Cursor, state.BytesUsed, state.CaptureLimited
	if filter.Until == 0 || filter.Until > state.Cursor {
		filter.Until = state.Cursor
	}
	records, err := s.readCachedRecords(ctx, workspace, filter, 21, false)
	if err != nil {
		return page, state, err
	}
	if len(records) > 20 {
		records = records[:20]
		if filter.Descending {
			page.NextBefore = records[19].Seq
		} else {
			page.NextAfter = records[19].Seq
		}
	}
	page.Records = append(page.Records, records...)
	return page, state, nil
}

func (s *Store) CachedMessages(ctx context.Context, workspace, episode string, limit int) ([]shared.HistoryRecord, error) {
	state, err := s.HistoryCacheState(ctx, workspace)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.readCachedRecords(ctx, workspace, HistoryFilter{Episode: episode, Until: state.Cursor}, limit, true)
}

func (s *Store) CachedRunEnded(ctx context.Context, workspace, episode string) (bool, error) {
	var ended bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM history_cache
		WHERE workspace_id=$1 AND episode_id=$2 AND kind IN ('run.completed','run.failed','run.cancelled'))`, workspace, episode).Scan(&ended)
	return ended, err
}

func (s *Store) readCachedRecords(ctx context.Context, workspace string, filter HistoryFilter, limit int, messagesOnly bool) ([]shared.HistoryRecord, error) {
	order := "ASC"
	if filter.Descending {
		order = "DESC"
	}
	query := `SELECT body FROM history_cache WHERE workspace_id=$1 AND seq>$2 AND seq<=$3 AND (CAST($4 AS BIGINT)=0 OR seq<$4) AND ($5='' OR episode_id=$5)`
	if messagesOnly {
		query += ` AND kind IN ('user.message','assistant.message')`
	}
	rows, err := s.DB.QueryContext(ctx, query+` ORDER BY seq `+order+` LIMIT $6`, workspace, filter.After, filter.Until, filter.Before, filter.Episode, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []shared.HistoryRecord
	for rows.Next() {
		var body string
		var record shared.HistoryRecord
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(body), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

type AuditEntry struct {
	Cursor      int64                `json:"cursor"`
	WorkspaceID string               `json:"workspace_id"`
	Record      shared.HistoryRecord `json:"record"`
}

func (s *Store) ProjectAudit(ctx context.Context, project, sharedWorker, worker, kind string, before int64) ([]AuditEntry, int64, error) {
	// Shared workers have one runtime owner. Attribute only episodes linked to
	// a work attempt by the control plane. Guest records cannot claim projects.
	taskID := "a.metadata ->> 'task_id'"
	if _, postgres := s.DB.Driver().(*stdlib.Driver); postgres {
		taskID = "CAST(a.metadata AS json) ->> 'task_id'"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT h.audit_seq,h.workspace_id,h.body,p.purpose
		FROM history_cache h LEFT JOIN history_purpose p ON p.workspace_id=h.workspace_id AND p.episode_id=h.episode_id
		WHERE ((h.project_id=$1 AND h.workspace_id<>$2) OR (h.workspace_id=$2 AND EXISTS (
			SELECT 1 FROM work_attempts a WHERE a.project_id=$1 AND a.workspace_id=h.workspace_id AND (`+taskID+`)=h.episode_id)))
		AND ($3='' OR h.workspace_id=$3) AND ($4='' OR h.kind=$4) AND (CAST($5 AS BIGINT)=0 OR h.audit_seq<$5)
		ORDER BY h.audit_seq DESC LIMIT 21`, project, sharedWorker, worker, kind, before)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []AuditEntry{}
	for rows.Next() {
		var item AuditEntry
		var body string
		var purpose sql.NullString
		if err := rows.Scan(&item.Cursor, &item.WorkspaceID, &body, &purpose); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal([]byte(body), &item.Record); err != nil {
			return nil, 0, err
		}
		item.Record.Purpose = purpose.String
		items = append(items, item)
	}
	var next int64
	if len(items) > 20 {
		items = items[:20]
		next = items[19].Cursor
	}
	return items, next, rows.Err()
}
