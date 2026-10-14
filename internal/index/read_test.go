package index

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

func cardJSON(name, description string) string {
	return fmt.Sprintf(`{"name":%q,"description":%q,"supportedInterfaces":[{"url":"https://x/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`, name, description)
}

func check(t *testing.T, i *Index, host string, at int64, card string) {
	t.Helper()
	o := newObservation(t, KindCard, nil)
	o.Host, o.At = host, time.Unix(at, 0)
	if card == "" {
		o.OK, o.HTTPStatus, o.Error = false, 404, "card request: status 404"
	} else {
		o.Card = []byte(card)
	}
	ingest(t, i, o)
}

func search(t *testing.T, i *Index, q Query) Results {
	t.Helper()
	if q.Status == "" {
		q.Status = StatusAny
	}
	if q.Limit == 0 {
		q.Limit = MaxLimit
	}
	r, err := i.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search(%+v): %v", q, err)
	}
	return r
}

func hostsOf(r Results) []string {
	var hosts []string
	for _, s := range r.Agents {
		hosts = append(hosts, s.Host)
	}
	return hosts
}

func seedHosts(t *testing.T) *Index {
	t.Helper()
	i, _ := openTemp(t)
	check(t, i, "b.example", 100, cardJSON("Beta", "Translates text"))
	check(t, i, "a.example", 100, cardJSON("alpha", "Echoes 100%"))
	check(t, i, "dead.example", 100, "")
	check(t, i, "was-up.example", 100, cardJSON("Gamma", "Was up"))
	check(t, i, "was-up.example", 200, "")
	return i
}

func TestSearchSummary(t *testing.T) {
	i := seedHosts(t)
	r := search(t, i, Query{Text: "was-up"})
	want := Summary{
		Host: "was-up.example", Name: "Gamma", Description: "Was up", ProtocolVersion: "1.0",
		HTTPStatus: 404, Error: "card request: status 404", LastChecked: time.Unix(200, 0),
	}
	if r.Total != 1 || len(r.Agents) != 1 || r.Agents[0] != want {
		t.Errorf("Search = %+v, want one result %+v", r, want)
	}
}

func TestSearchFilters(t *testing.T) {
	i := seedHosts(t)
	tests := []struct {
		name  string
		query Query
		want  []string
	}{
		{name: "all, named first", query: Query{}, want: []string{"a.example", "b.example", "was-up.example", "dead.example"}},
		{name: "ok", query: Query{Status: StatusOK}, want: []string{"a.example", "b.example"}},
		{name: "failed", query: Query{Status: StatusFailed}, want: []string{"was-up.example", "dead.example"}},
		{name: "host", query: Query{Text: "dead"}, want: []string{"dead.example"}},
		{name: "name, any case", query: Query{Text: "BETA"}, want: []string{"b.example"}},
		{name: "description", query: Query{Text: "translates"}, want: []string{"b.example"}},
		{name: "percent is literal", query: Query{Text: "100%"}, want: []string{"a.example"}},
		{name: "underscore is literal", query: Query{Text: "_"}},
		{name: "limit", query: Query{Limit: 2}, want: []string{"a.example", "b.example"}},
		{name: "offset", query: Query{Limit: 2, Offset: 3}, want: []string{"dead.example"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hostsOf(search(t, i, tt.query)); !slices.Equal(got, tt.want) {
				t.Errorf("hosts = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSearchTotal(t *testing.T) {
	i := seedHosts(t)
	if r := search(t, i, Query{Limit: 1}); r.Total != 4 {
		t.Errorf("Total = %d, want 4", r.Total)
	}
}

func TestSearchIgnoresObserversWithoutWeight(t *testing.T) {
	i, _ := openTemp(t)
	if _, err := i.db.ExecContext(t.Context(), `INSERT INTO observers (observer, weight) VALUES ('other', 0)`); err != nil {
		t.Fatal(err)
	}
	o := newObservation(t, KindCard, []byte(cardJSON("A", "")))
	o.Observer = "other"
	ingest(t, i, o)
	if r := search(t, i, Query{}); r.Total != 0 {
		t.Errorf("Total = %d, want 0", r.Total)
	}
}

func TestSearchInvalidQuery(t *testing.T) {
	i, _ := openTemp(t)
	for _, q := range []Query{
		{Status: "live", Limit: 10},
		{Status: StatusAny, Limit: 0},
		{Status: StatusAny, Limit: MaxLimit + 1},
		{Status: StatusAny, Limit: 10, Offset: -1},
	} {
		if _, err := i.Search(context.Background(), q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("Search(%+v) error = %v, want ErrInvalidQuery", q, err)
		}
	}
}

func TestProtocolVersion(t *testing.T) {
	i, _ := openTemp(t)
	check(t, i, "v1.example", 100, cardJSON("V1", ""))
	check(t, i, "v03.example", 100, `{"name":"V03","protocolVersion":"0.3.0","url":"https://x/a2a"}`)
	got := map[string]string{}
	for _, s := range search(t, i, Query{}).Agents {
		got[s.Host] = s.ProtocolVersion
	}
	if got["v1.example"] != "1.0" || got["v03.example"] != "0.3.0" {
		t.Errorf("protocol versions = %v, want v1.example 1.0, v03.example 0.3.0", got)
	}
}

func TestAgent(t *testing.T) {
	i := seedHosts(t)
	for at := int64(300); at < 330; at++ {
		check(t, i, "was-up.example", at, "")
	}
	a, err := i.Agent(context.Background(), "was-up.example")
	if err != nil {
		t.Fatalf("Agent: %v", err)
	}
	var card struct{ Name string }
	if err := json.Unmarshal(a.Card, &card); err != nil || card.Name != "Gamma" {
		t.Errorf("Card = %s, want the card of Gamma", a.Card)
	}
	if len(a.Observations) != agentObservations {
		t.Fatalf("observations = %d, want %d", len(a.Observations), agentObservations)
	}
	if first := a.Observations[0]; first.At != time.Unix(329, 0) || first.OK || first.Latency != 120*time.Millisecond {
		t.Errorf("first observation = %+v, want the failed check at 329 with latency 120ms", first)
	}
}

func TestAgentWithoutCard(t *testing.T) {
	i := seedHosts(t)
	a, err := i.Agent(context.Background(), "dead.example")
	if err != nil {
		t.Fatalf("Agent: %v", err)
	}
	if a.Card != nil || len(a.Observations) != 1 {
		t.Errorf("Agent = %+v, want no card and 1 observation", a)
	}
}

func TestAgentNotFound(t *testing.T) {
	i := seedHosts(t)
	if _, err := i.Agent(context.Background(), "unknown.example"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestOpenReadOnly(t *testing.T) {
	w, path := openTemp(t)
	check(t, w, "a.example", 100, cardJSON("A", ""))

	r, err := OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if got := hostsOf(search(t, r, Query{})); !slices.Equal(got, []string{"a.example"}) {
		t.Errorf("hosts = %q, want [a.example]", got)
	}
	o := newObservation(t, KindCard, nil)
	o.Observer = w.NodeID()
	if err := r.Ingest(context.Background(), o); err == nil {
		t.Error("Ingest on a read-only index: no error")
	}
}

func TestOpenReadOnlyMissingFile(t *testing.T) {
	if _, err := OpenReadOnly(context.Background(), t.TempDir()+"/missing.db"); err == nil {
		t.Error("OpenReadOnly of a missing file: no error")
	}
}
