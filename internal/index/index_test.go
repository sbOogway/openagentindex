package index

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func openTemp(t *testing.T) (*Index, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.db")
	i, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = i.Close() })
	return i, path
}

func newObservation(t *testing.T, kind string, card []byte) Observation {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return Observation{
		ID:         id.String(),
		Host:       "agent.example.com",
		Kind:       kind,
		At:         time.Unix(1792000000, 0),
		OK:         true,
		HTTPStatus: 200,
		Latency:    120 * time.Millisecond,
		Card:       card,
	}
}

func ingest(t *testing.T, i *Index, o Observation) {
	t.Helper()
	if err := i.Ingest(context.Background(), o); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
}

func count(t *testing.T, i *Index, table string) int {
	t.Helper()
	var n int
	if err := i.db.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func cardHashOf(t *testing.T, i *Index, id string) sql.NullString {
	t.Helper()
	var h sql.NullString
	if err := i.db.QueryRowContext(t.Context(), `SELECT card_hash FROM observations WHERE id = ?`, id).Scan(&h); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestOpenCreatesSchema(t *testing.T) {
	i, _ := openTemp(t)
	for _, table := range []string{"node", "observers", "cards", "observations"} {
		var name string
		err := i.db.QueryRowContext(t.Context(), `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %s: %v", table, err)
		}
	}
	var mode string
	var foreignKeys int
	if err := i.db.QueryRowContext(t.Context(), `PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, %v; want wal", mode, err)
	}
	if err := i.db.QueryRowContext(t.Context(), `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, %v; want 1", foreignKeys, err)
	}
}

func TestNodeIDIsStable(t *testing.T) {
	i, path := openTemp(t)
	id, err := uuid.Parse(i.NodeID())
	if err != nil || id.Version() != 7 {
		t.Fatalf("node ID %q is not a UUIDv7", i.NodeID())
	}
	var weight float64
	if err := i.db.QueryRowContext(t.Context(), `SELECT weight FROM observers WHERE observer = ?`, i.NodeID()).Scan(&weight); err != nil || weight != 1 {
		t.Errorf("observer weight = %v, %v; want 1", weight, err)
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if reopened.NodeID() != id.String() {
		t.Errorf("node ID after reopen = %q, want %q", reopened.NodeID(), id)
	}
	if n := count(t, reopened, "observers"); n != 1 {
		t.Errorf("observers = %d, want 1", n)
	}
}

func TestReopenAppliesNoMigration(t *testing.T) {
	i, path := openTemp(t)
	if v := schemaVersion(t, i); v != 1 {
		t.Fatalf("schema version = %d, want 1", v)
	}
	applied := count(t, i, "goose_db_version")
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if n := count(t, reopened, "goose_db_version"); n != applied {
		t.Errorf("goose_db_version rows after reopen = %d, want %d", n, applied)
	}
}

func schemaVersion(t *testing.T, i *Index) int64 {
	t.Helper()
	var v int64
	if err := i.db.QueryRowContext(t.Context(), `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestIngestIsIdempotent(t *testing.T) {
	i, _ := openTemp(t)
	o := newObservation(t, KindCard, []byte(`{"name":"A"}`))
	ingest(t, i, o)
	ingest(t, i, o)
	if n := count(t, i, "observations"); n != 1 {
		t.Errorf("observations = %d, want 1", n)
	}
	if n := count(t, i, "cards"); n != 1 {
		t.Errorf("cards = %d, want 1", n)
	}
}

func TestEqualCardsShareOneHash(t *testing.T) {
	i, _ := openTemp(t)
	a := newObservation(t, KindCard, []byte(`{"b":1,"a":2}`))
	b := newObservation(t, KindCard, []byte("{ \"a\": 2,\n  \"b\": 1 }"))
	ingest(t, i, a)
	ingest(t, i, b)

	sum := sha256.Sum256([]byte(`{"a":2,"b":1}`))
	want := hex.EncodeToString(sum[:])
	for _, o := range []Observation{a, b} {
		if h := cardHashOf(t, i, o.ID); h.String != want {
			t.Errorf("card_hash = %q, want %q", h.String, want)
		}
	}
	if n := count(t, i, "cards"); n != 1 {
		t.Errorf("cards = %d, want 1", n)
	}
}

func TestEndpointObservationWithoutCard(t *testing.T) {
	i, _ := openTemp(t)
	o := newObservation(t, KindEndpoint, nil)
	ingest(t, i, o)
	if h := cardHashOf(t, i, o.ID); h.Valid {
		t.Errorf("card_hash = %q, want NULL", h.String)
	}
	if n := count(t, i, "cards"); n != 0 {
		t.Errorf("cards = %d, want 0", n)
	}
}

func TestIngestRejectsInvalid(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Observation)
	}{
		{name: "empty ID", modify: func(o *Observation) { o.ID = "" }},
		{name: "empty host", modify: func(o *Observation) { o.Host = "" }},
		{name: "unknown kind", modify: func(o *Observation) { o.Kind = "ping" }},
		{name: "card not JSON", modify: func(o *Observation) { o.Card = []byte("<html>") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i, _ := openTemp(t)
			o := newObservation(t, KindCard, []byte(`{"name":"A"}`))
			tt.modify(&o)
			if err := i.Ingest(context.Background(), o); !errors.Is(err, ErrInvalidObservation) {
				t.Errorf("err = %v, want ErrInvalidObservation", err)
			}
			if n := count(t, i, "observations"); n != 0 {
				t.Errorf("observations = %d, want 0", n)
			}
		})
	}
}
