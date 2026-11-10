package coordination

import (
	"context"
	"database/sql"
	"errors"
	_ "modernc.org/sqlite"
	"os"
	"sync"
	"testing"
)

func setup(t *testing.T) (*Service, context.Context) {
	t.Helper()
	db, e := sql.Open("sqlite", t.TempDir()+"/metadata.db")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	if _, e = db.Exec(Schema); e != nil {
		t.Fatal(e)
	}
	stores := map[string]BlobStore{}
	for _, p := range []string{"source", "personal"} {
		r, e := os.OpenRoot(t.TempDir())
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { r.Close() })
		stores[p] = Files{r}
	}
	return &Service{DB: db, Blobs: func(p string) (BlobStore, error) {
		b, ok := stores[p]
		if !ok {
			return nil, ErrDenied
		}
		return b, nil
	}}, context.Background()
}
func TestBilateralRevocationAndIsolation(t *testing.T) {
	s, c := setup(t)
	g := Grant{"grant", "source", "personal", "summary"}
	if e := s.Create(c, "source", g); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Publish(c, "source", "grant", "daily", []byte("selected")); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	if e := s.Approve(c, "foreign", "grant"); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	if e := s.Approve(c, "personal", "grant"); e != nil {
		t.Fatal(e)
	}
	x, e := s.Publish(c, "source", "grant", "daily", []byte("selected"))
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Publish(c, "source", "grant", "daily", []byte("selected"))
	if e != nil || again != x {
		t.Fatal(e)
	}
	if _, e = s.Publish(c, "source", "grant", "daily", []byte("changed")); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if _, _, e = s.Read(c, "foreign", x.ID); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	if _, b, e := s.Read(c, "personal", x.ID); e != nil || string(b) != "selected" {
		t.Fatal(e)
	}
	if e = s.Revoke(c, "personal", "grant"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Read(c, "personal", x.ID); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	if _, e = s.Publish(c, "source", "grant", "next", []byte("secret")); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	if e = s.Approve(c, "personal", "grant"); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
}
func TestNotesRetryConflictAndConcurrentWrites(t *testing.T) {
	r, e := os.OpenRoot(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	n := Notes{r}
	in := NoteWrite{"daily.md", "Summary", "", "daily", []string{"https://example.org/source"}}
	first, e := n.Publish(in)
	if e != nil {
		t.Fatal(e)
	}
	if second, e := n.Publish(in); e != nil || second != first {
		t.Fatal(e)
	}
	if e = r.WriteFile("daily.md", []byte("human edit"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = n.Publish(in); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []string{"one", "two"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, e := n.Publish(NoteWrite{"concurrent.md", id, "", id, in.Sources})
			results <- e
		}(id)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	if _, e = n.Publish(NoteWrite{"../escape.md", "x", "", "escape", in.Sources}); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
