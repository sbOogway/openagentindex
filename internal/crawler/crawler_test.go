package crawler_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/uuid"

	"github.com/sbOogway/openagentindex/internal/crawler"
	"github.com/sbOogway/openagentindex/internal/index"
	"github.com/sbOogway/openagentindex/internal/probe"
)

var (
	errSource = errors.New("source down")
	errStore  = errors.New("disk full")
	at        = time.Unix(1792000000, 0)
)

type fakeSource struct {
	hosts []string
	err   error
}

func (f fakeSource) Hosts(context.Context) ([]string, error) {
	return f.hosts, f.err
}

type fakeProber struct {
	results map[string]probe.Result
	checked []string
	onCheck func()
}

func (f *fakeProber) CheckCard(_ context.Context, host string) probe.Result {
	f.checked = append(f.checked, host)
	if f.onCheck != nil {
		f.onCheck()
	}
	if r, ok := f.results[host]; ok {
		return r
	}
	return probe.Result{Host: host, Kind: probe.KindCard, At: at, HTTPStatus: 404, Err: errors.New("card request: status 404")}
}

type fakeStore struct {
	stored []index.Observation
	err    error
}

func (f *fakeStore) Ingest(_ context.Context, o index.Observation) error {
	if f.err != nil {
		return f.err
	}
	f.stored = append(f.stored, o)
	return nil
}

func newCrawler(p crawler.Prober, s crawler.Store, sources ...crawler.Source) *crawler.Crawler {
	return crawler.New(crawler.Config{
		Sources: sources,
		Prober:  p,
		Store:   s,
		Log:     slog.New(slog.DiscardHandler),
	})
}

func storedHosts(s *fakeStore) []string {
	var hosts []string
	for _, o := range s.stored {
		hosts = append(hosts, o.Host)
	}
	return hosts
}

func TestRunChecksEachHostOnce(t *testing.T) {
	p, s := &fakeProber{}, &fakeStore{}
	c := newCrawler(p, s,
		fakeSource{hosts: []string{"a.example", "b.example"}},
		fakeSource{hosts: []string{"b.example", "c.example"}},
	)
	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"a.example", "b.example", "c.example"}
	if !slices.Equal(p.checked, want) {
		t.Errorf("checked = %q, want %q", p.checked, want)
	}
	if got := storedHosts(s); !slices.Equal(got, want) {
		t.Errorf("stored = %q, want %q", got, want)
	}
}

func TestRunConvertsResults(t *testing.T) {
	raw := []byte(`{"name":"A"}`)
	tests := []struct {
		name   string
		result probe.Result
		want   index.Observation
	}{
		{
			name: "valid card",
			result: probe.Result{
				Host: "a.example", Kind: probe.KindCard, At: at, OK: true,
				HTTPStatus: 200, Latency: 120 * time.Millisecond,
				Card: &a2a.AgentCard{Name: "A"}, Raw: raw,
			},
			want: index.Observation{
				Host: "a.example", Kind: index.KindCard, At: at, OK: true,
				HTTPStatus: 200, Latency: 120 * time.Millisecond, Card: raw,
			},
		},
		{
			name: "failed check",
			result: probe.Result{
				Host: "a.example", Kind: probe.KindCard, At: at,
				HTTPStatus: 500, Latency: time.Second, Err: errors.New("card request: status 500"),
			},
			want: index.Observation{
				Host: "a.example", Kind: index.KindCard, At: at,
				HTTPStatus: 500, Latency: time.Second, Error: "card request: status 500",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &fakeProber{results: map[string]probe.Result{"a.example": tt.result}}
			s := &fakeStore{}
			if err := newCrawler(p, s, fakeSource{hosts: []string{"a.example"}}).Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(s.stored) != 1 {
				t.Fatalf("stored %d observations, want 1", len(s.stored))
			}
			got := s.stored[0]
			if id, err := uuid.Parse(got.ID); err != nil || id.Version() != 7 {
				t.Errorf("ID = %q, want a UUIDv7", got.ID)
			}
			got.ID = ""
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("observation = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRunSkipsBadHost(t *testing.T) {
	p := &fakeProber{results: map[string]probe.Result{
		"bad/host": {Host: "bad/host", Kind: probe.KindCard, At: at, Err: fmt.Errorf("%w: %q", probe.ErrBadHost, "bad/host")},
	}}
	s := &fakeStore{}
	if err := newCrawler(p, s, fakeSource{hosts: []string{"bad/host", "a.example"}}).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := storedHosts(s); !slices.Equal(got, []string{"a.example"}) {
		t.Errorf("stored = %q, want [a.example]", got)
	}
}

func TestRunUsesOtherSourcesWhenOneFails(t *testing.T) {
	p, s := &fakeProber{}, &fakeStore{}
	c := newCrawler(p, s, fakeSource{err: errSource}, fakeSource{hosts: []string{"a.example"}})
	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !slices.Equal(p.checked, []string{"a.example"}) {
		t.Errorf("checked = %q, want [a.example]", p.checked)
	}
}

func TestRunStopsWhenIngestFails(t *testing.T) {
	p, s := &fakeProber{}, &fakeStore{err: errStore}
	err := newCrawler(p, s, fakeSource{hosts: []string{"a.example", "b.example"}}).Run(context.Background())
	if !errors.Is(err, errStore) {
		t.Errorf("Run error = %v, want %v", err, errStore)
	}
	if len(p.checked) != 1 {
		t.Errorf("checked %d hosts, want 1", len(p.checked))
	}
}

func TestRunStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, s := &fakeProber{onCheck: cancel}, &fakeStore{}
	err := newCrawler(p, s, fakeSource{hosts: []string{"a.example", "b.example"}}).Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run error = %v, want context.Canceled", err)
	}
	if !slices.Equal(p.checked, []string{"a.example"}) {
		t.Errorf("checked = %q, want [a.example]", p.checked)
	}
}
