package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	StatusOK     = "ok"
	StatusFailed = "failed"
	StatusAny    = "any"

	MaxLimit    = 100
	MaxTags     = 10
	MaxTagLimit = 200

	agentObservations = 20
)

var (
	bindings = []string{"JSONRPC", "HTTP+JSON", "GRPC"}
	versions = []string{"1.0", "0.3"}
)

var (
	ErrNotFound     = errors.New("host not found")
	ErrInvalidQuery = errors.New("invalid query")
)

// Query selects hosts. Text matches the host, name or description.
// Status is StatusOK, StatusFailed or StatusAny. It uses the newest card check of each host.
// An empty Binding or Version matches all hosts. A host must have all Tags (any case).
type Query struct {
	Text    string
	Status  string
	Tags    []string
	Binding string
	Version string
	Limit   int
	Offset  int
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
	Tags            []string
	Bindings        []string
}

type Results struct {
	Total  int
	Agents []Summary
}

type TagCount struct {
	Tag   string
	Hosts int
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

func (i *Index) Search(ctx context.Context, q Query) (Results, error) {
	if err := validateQuery(q); err != nil {
		return Results{}, err
	}
	tags, err := json.Marshal(normalizeTags(q.Tags))
	if err != nil {
		return Results{}, err
	}
	rows, err := i.db.QueryContext(ctx, querySearch,
		sql.Named("status", q.Status), sql.Named("text", likePattern(q.Text)),
		sql.Named("binding", q.Binding), sql.Named("version", q.Version), sql.Named("tags", string(tags)),
		sql.Named("limit", q.Limit), sql.Named("offset", q.Offset))
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
	err := i.db.QueryRowContext(ctx, queryAgent, sql.Named("host", host)).
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
	rows, err := i.db.QueryContext(ctx, queryObservations,
		sql.Named("host", host), sql.Named("limit", agentObservations))
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

// Tags returns the most common tags of the hosts with a current card. Prefix filters the tags.
func (i *Index) Tags(ctx context.Context, prefix string, limit int) ([]TagCount, error) {
	if limit < 1 || limit > MaxTagLimit {
		return nil, fmt.Errorf("%w: limit %d is not 1 to %d", ErrInvalidQuery, limit, MaxTagLimit)
	}
	pattern := ""
	if p := strings.ToLower(strings.TrimSpace(prefix)); p != "" {
		pattern = escapeLike(p) + "%"
	}
	rows, err := i.db.QueryContext(ctx, queryTags, sql.Named("prefix", pattern), sql.Named("limit", limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TagCount{}
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Hosts); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func summaryFields(s *Summary) []any {
	return []any{
		&s.Host, &s.Name, &s.Description, &s.ProtocolVersion, &s.OK, &s.HTTPStatus, &s.Error,
		unixTime{&s.LastChecked}, jsonStrings{&s.Tags}, jsonStrings{&s.Bindings},
	}
}

// jsonStrings scans a JSON array of strings.
type jsonStrings struct{ s *[]string }

func (j jsonStrings) Scan(v any) error {
	text, ok := v.(string)
	if !ok {
		return fmt.Errorf("JSON array: unexpected type %T", v)
	}
	return json.Unmarshal([]byte(text), j.s)
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
	case q.Binding != "" && !slices.Contains(bindings, q.Binding):
		return fmt.Errorf("%w: binding %q is not one of %s", ErrInvalidQuery, q.Binding, strings.Join(bindings, ", "))
	case q.Version != "" && !slices.Contains(versions, q.Version):
		return fmt.Errorf("%w: version %q is not one of %s", ErrInvalidQuery, q.Version, strings.Join(versions, ", "))
	case len(q.Tags) > MaxTags:
		return fmt.Errorf("%w: more than %d tags", ErrInvalidQuery, MaxTags)
	case slices.Contains(normalizeTags(q.Tags), ""):
		return fmt.Errorf("%w: empty tag", ErrInvalidQuery)
	}
	return nil
}

func normalizeTags(tags []string) []string {
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = strings.ToLower(strings.TrimSpace(t))
	}
	return out
}

func likePattern(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return "%" + escapeLike(text) + "%"
}

func escapeLike(text string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text)
}
