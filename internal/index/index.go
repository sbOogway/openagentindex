package index

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

const (
	KindCard     = "card"
	KindEndpoint = "endpoint"
)

var ErrInvalidObservation = errors.New("invalid observation")

//go:embed schema.sql
var schema string

type Index struct {
	db     *sql.DB
	nodeID string
}

// Observation is the record of one check. Card is the raw card JSON, or nil.
// An empty Observer means this node.
type Observation struct {
	ID         string
	Observer   string
	Host       string
	Kind       string
	At         time.Time
	OK         bool
	HTTPStatus int
	Latency    time.Duration
	Error      string
	Card       []byte
}

func Open(ctx context.Context, path string) (*Index, error) {
	dsn := "file:" + path + "?_txlock=immediate" +
		"&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return nil, errors.Join(fmt.Errorf("create schema: %w", err), db.Close())
	}
	nodeID, err := ensureNode(ctx, db)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create node: %w", err), db.Close())
	}
	return &Index{db: db, nodeID: nodeID}, nil
}

func (i *Index) Close() error {
	return i.db.Close()
}

func (i *Index) NodeID() string {
	return i.nodeID
}

// Ingest stores the observation and its card. It ignores an observation or a
// card that is already stored.
func (i *Index) Ingest(ctx context.Context, o Observation) error {
	if err := validate(o); err != nil {
		return err
	}
	if o.Observer == "" {
		o.Observer = i.nodeID
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	cardHash, err := insertCard(ctx, tx, o)
	if err != nil {
		return err
	}
	if err := insertObservation(ctx, tx, o, cardHash); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureNode(ctx context.Context, db *sql.DB) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM node`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	v7, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	id = v7.String()
	if _, err := tx.ExecContext(ctx, `INSERT INTO node (id, created_at) VALUES (?, ?)`, id, time.Now().Unix()); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO observers (observer, weight) VALUES (?, 1)`, id); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func validate(o Observation) error {
	switch {
	case o.ID == "":
		return fmt.Errorf("%w: no ID", ErrInvalidObservation)
	case o.Host == "":
		return fmt.Errorf("%w: no host", ErrInvalidObservation)
	case o.Kind != KindCard && o.Kind != KindEndpoint:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidObservation, o.Kind)
	}
	return nil
}

func insertCard(ctx context.Context, tx *sql.Tx, o Observation) (sql.NullString, error) {
	if o.Card == nil {
		return sql.NullString{}, nil
	}
	canonical, err := jcs.Transform(o.Card)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("%w: card: %w", ErrInvalidObservation, err)
	}
	sum := sha256.Sum256(canonical)
	hash := hex.EncodeToString(sum[:])
	_, err = tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO cards (card_hash, card_json, first_seen) VALUES (?, ?, ?)`,
		hash, string(canonical), o.At.Unix())
	return sql.NullString{String: hash, Valid: true}, err
}

func insertObservation(ctx context.Context, tx *sql.Tx, o Observation, cardHash sql.NullString) error {
	_, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO observations
		 (id, observer, host, kind, at, ok, card_hash, http_status, latency_ms, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.ID, o.Observer, o.Host, o.Kind, o.At.Unix(), o.OK, cardHash,
		o.HTTPStatus, o.Latency.Milliseconds(), o.Error)
	return err
}
