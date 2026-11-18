// Package jsonmetadata reads SQL rows containing JSON metadata.
package jsonmetadata

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Query reads one metadata column per row in query order.
// Query, scan, and decode failures return a nil slice. An iteration error
// returns the items read so far. A successful empty result is non-nil.
func Query[T any](ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, query string, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []T{}
	for rows.Next() {
		var item T
		if err := Scan(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Get reads one metadata column from a single row.
// Scan failures return the zero value. Decode failures may return a partial value.
func Get[T any](ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, args ...any) (T, error) {
	var item T
	err := Scan(db.QueryRowContext(ctx, query, args...), &item)
	return item, err
}

// Scan decodes one metadata column into dst after a successful scan.
func Scan[T any](row interface{ Scan(...any) error }, dst *T) error {
	var metadata string
	if err := row.Scan(&metadata); err != nil {
		return err
	}
	return json.Unmarshal([]byte(metadata), dst)
}
