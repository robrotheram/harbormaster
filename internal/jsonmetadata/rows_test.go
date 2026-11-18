package jsonmetadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

type testMetadata struct {
	ID    string
	Count int
}

func TestQuery(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	items, err := Query[testMetadata](ctx, db, `SELECT '{"ID":"first"}' UNION ALL SELECT '{"ID":"second"}'`)
	if err != nil || len(items) != 2 || items[0].ID != "first" || items[1].ID != "second" {
		t.Fatalf("ordered query = %v, %v", items, err)
	}

	items, err = Query[testMetadata](ctx, db, `SELECT NULL WHERE 1=0`)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("empty query = %v, %v", items, err)
	}

	items, err = Query[testMetadata](ctx, db, `SELECT '{}' UNION ALL SELECT NULL`)
	if items != nil || err == nil {
		t.Fatalf("query with a scan error = %v, %v", items, err)
	}

	items, err = Query[testMetadata](ctx, db, `SELECT '{}' UNION ALL SELECT '{'`)
	var syntaxErr *json.SyntaxError
	if items != nil || !errors.As(err, &syntaxErr) {
		t.Fatalf("query with malformed JSON = %v, %v", items, err)
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	item, err := Get[testMetadata](ctx, db, `SELECT NULL WHERE 1=0`)
	if item != (testMetadata{}) || err != sql.ErrNoRows {
		t.Fatalf("missing row = %v, %v", item, err)
	}

	item, err = Get[testMetadata](ctx, db, `SELECT $1`, `{"ID":"partial","Count":"invalid"}`)
	var typeErr *json.UnmarshalTypeError
	if item != (testMetadata{ID: "partial"}) || !errors.As(err, &typeErr) {
		t.Fatalf("row with a JSON type error = %v, %v", item, err)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
