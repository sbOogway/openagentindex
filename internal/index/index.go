package index

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

const (
	KindCard     = "card"
	KindEndpoint = "endpoint"
)

var ErrInvalidObservation = errors.New("invalid observation")

//go:embed migrations/*.sql
var migrations embed.FS

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
	if err := migrate(ctx, db); err != nil {
		return nil, errors.Join(fmt.Errorf("migrate: %w", err), db.Close())
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

func migrate(ctx context.Context, db *sql.DB) error {
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, fsys)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}

func ensureNode(ctx context.Context, db *sql.DB) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	err = tx.QueryRowContext(ctx, queryNodeSelect).Scan(&id)
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
	if _, err := tx.ExecContext(ctx, queryNodeInsert, sql.Named("id", id), sql.Named("created_at", time.Now().Unix())); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, queryObserverInsert, sql.Named("observer", id), sql.Named("weight", 1)); err != nil {
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
	_, err = tx.ExecContext(ctx, queryCardInsert,
		sql.Named("card_hash", hash), sql.Named("card_json", string(canonical)), sql.Named("first_seen", o.At.Unix()))
	return sql.NullString{String: hash, Valid: true}, err
}

func insertObservation(ctx context.Context, tx *sql.Tx, o Observation, cardHash sql.NullString) error {
	_, err := tx.ExecContext(ctx, queryObservationInsert,
		sql.Named("id", o.ID), sql.Named("observer", o.Observer), sql.Named("host", o.Host),
		sql.Named("kind", o.Kind), sql.Named("at", o.At.Unix()), sql.Named("ok", o.OK),
		sql.Named("card_hash", cardHash), sql.Named("http_status", o.HTTPStatus),
		sql.Named("latency_ms", o.Latency.Milliseconds()), sql.Named("error", o.Error))
	return err
}
