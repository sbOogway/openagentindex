package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	StatusOK     = "ok"
	StatusFailed = "failed"
	StatusAny    = "any"

	MaxLimit = 100

	agentObservations = 20
)

var (
	ErrNotFound     = errors.New("host not found")
	ErrInvalidQuery = errors.New("invalid query")
)

// Query selects hosts. Text matches the host, name or description.
// Status is StatusOK, StatusFailed or StatusAny. It uses the newest card check of each host.
type Query struct {
	Text   string
	Status string
	Limit  int
	Offset int
}

// Summary is the newest card check of a host and the fields of its current card.
type Summary struct {
	Host            string
	Name            string
	Description     string
	ProtocolVersion string
	OK              bool
	HTTPStatus      int
	Error           string
	LastChecked     time.Time
}

type Results struct {
	Total  int
	Agents []Summary
}

// Agent is a host with its current card (nil if none) and its newest observations.
type Agent struct {
	Summary
	Card         json.RawMessage
	Observations []Observation
}

// OpenReadOnly opens an index that another process writes. It does not apply migrations.
func OpenReadOnly(ctx context.Context, path string) (*Index, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return &Index{db: db}, nil
}

const summaryColumns = `host, name, description, protocol_version, ok, http_status, error, at`

func (i *Index) Search(ctx context.Context, q Query) (Results, error) {
	if err := validateQuery(q); err != nil {
		return Results{}, err
	}
	rows, err := i.db.QueryContext(ctx, `
		SELECT `+summaryColumns+`, COUNT(*) OVER ()
		FROM host_summaries
		WHERE (?1 = 'any' OR (?1 = 'ok') = (ok = 1))
		  AND (?2 = '' OR host LIKE ?2 ESCAPE '\' OR name LIKE ?2 ESCAPE '\' OR description LIKE ?2 ESCAPE '\')
		ORDER BY name = '', lower(name), host
		LIMIT ?3 OFFSET ?4`,
		q.Status, likePattern(q.Text), q.Limit, q.Offset)
	if err != nil {
		return Results{}, err
	}
	defer rows.Close()

	var r Results
	for rows.Next() {
		var s Summary
		if err := rows.Scan(append(summaryFields(&s), &r.Total)...); err != nil {
			return Results{}, err
		}
		r.Agents = append(r.Agents, s)
	}
	return r, rows.Err()
}

func (i *Index) Agent(ctx context.Context, host string) (Agent, error) {
	var a Agent
	var card sql.NullString
	err := i.db.QueryRowContext(ctx, `
		SELECT `+summaryColumns+`, card_json FROM host_summaries WHERE host = ?`, host).
		Scan(append(summaryFields(&a.Summary), &card)...)
	if errors.Is(err, sql.ErrNoRows) {
		return Agent{}, fmt.Errorf("%w: %s", ErrNotFound, host)
	}
	if err != nil {
		return Agent{}, err
	}
	if card.Valid {
		a.Card = json.RawMessage(card.String)
	}
	a.Observations, err = i.observations(ctx, host)
	return a, err
}

func (i *Index) observations(ctx context.Context, host string) ([]Observation, error) {
	rows, err := i.db.QueryContext(ctx, `
		SELECT id, observer, kind, at, ok, http_status, latency_ms, error
		FROM observations WHERE host = ? ORDER BY at DESC, id DESC LIMIT ?`, host, agentObservations)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Observation
	for rows.Next() {
		o := Observation{Host: host}
		var at, latencyMS int64
		if err := rows.Scan(&o.ID, &o.Observer, &o.Kind, &at, &o.OK, &o.HTTPStatus, &latencyMS, &o.Error); err != nil {
			return nil, err
		}
		o.At, o.Latency = time.Unix(at, 0), time.Duration(latencyMS)*time.Millisecond
		out = append(out, o)
	}
	return out, rows.Err()
}

func summaryFields(s *Summary) []any {
	return []any{&s.Host, &s.Name, &s.Description, &s.ProtocolVersion, &s.OK, &s.HTTPStatus, &s.Error, unixTime{&s.LastChecked}}
}

type unixTime struct{ t *time.Time }

func (u unixTime) Scan(v any) error {
	sec, ok := v.(int64)
	if !ok {
		return fmt.Errorf("time: unexpected type %T", v)
	}
	*u.t = time.Unix(sec, 0)
	return nil
}

func validateQuery(q Query) error {
	switch {
	case q.Status != StatusOK && q.Status != StatusFailed && q.Status != StatusAny:
		return fmt.Errorf("%w: status %q", ErrInvalidQuery, q.Status)
	case q.Limit < 1 || q.Limit > MaxLimit:
		return fmt.Errorf("%w: limit %d is not 1 to %d", ErrInvalidQuery, q.Limit, MaxLimit)
	case q.Offset < 0:
		return fmt.Errorf("%w: offset %d", ErrInvalidQuery, q.Offset)
	}
	return nil
}

func likePattern(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text) + "%"
}
