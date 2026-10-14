package probe_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/sbOogway/openagentindex/internal/card"
	"github.com/sbOogway/openagentindex/internal/probe"
)

const (
	newURL    = "https://a.example/.well-known/agent-card.json"
	legacyURL = "https://a.example/.well-known/agent.json"
	validCard = `{"name": "A", "supportedInterfaces": [{"url": "https://a.example/a2a", "protocolBinding": "JSONRPC"}]}`
)

var errNetwork = errors.New("network down")

type response struct {
	status int
	body   string
	err    error
}

type fakeGetter struct {
	responses map[string]response
	requested []string
}

func (f *fakeGetter) Get(_ context.Context, url string) (int, []byte, error) {
	f.requested = append(f.requested, url)
	r, ok := f.responses[url]
	if !ok {
		return 404, nil, nil
	}
	return r.status, []byte(r.body), r.err
}

func TestCheckCard(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		responses map[string]response
		ok        bool
		status    int
		wantErr   error
		requested []string
	}{
		{
			name:      "card at agent-card.json",
			host:      "a.example",
			responses: map[string]response{newURL: {status: 200, body: validCard}},
			ok:        true,
			status:    200,
			requested: []string{newURL},
		},
		{
			name:      "404, then card at agent.json",
			host:      "a.example",
			responses: map[string]response{legacyURL: {status: 200, body: validCard}},
			ok:        true,
			status:    200,
			requested: []string{newURL, legacyURL},
		},
		{
			name:      "both paths 404",
			host:      "a.example",
			status:    404,
			requested: []string{newURL, legacyURL},
		},
		{
			name:      "500, no fallback",
			host:      "a.example",
			responses: map[string]response{newURL: {status: 500}},
			status:    500,
			requested: []string{newURL},
		},
		{
			name:      "invalid card",
			host:      "a.example",
			responses: map[string]response{newURL: {status: 200, body: `{"hello": "world"}`}},
			status:    200,
			wantErr:   card.ErrInvalid,
			requested: []string{newURL},
		},
		{
			name:      "getter error",
			host:      "a.example",
			responses: map[string]response{newURL: {err: errNetwork}},
			wantErr:   errNetwork,
			requested: []string{newURL},
		},
		{name: "host with path", host: "a.example/x", wantErr: probe.ErrBadHost},
		{name: "host with scheme", host: "https://a.example", wantErr: probe.ErrBadHost},
		{name: "host with user", host: "u@a.example", wantErr: probe.ErrBadHost},
		{name: "empty host", host: "", wantErr: probe.ErrBadHost},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &fakeGetter{responses: tt.responses}
			r := probe.New(g).CheckCard(context.Background(), tt.host)

			if r.OK != tt.ok {
				t.Errorf("OK = %v, want %v (err: %v)", r.OK, tt.ok, r.Err)
			}
			if r.HTTPStatus != tt.status {
				t.Errorf("HTTPStatus = %d, want %d", r.HTTPStatus, tt.status)
			}
			if tt.wantErr != nil && !errors.Is(r.Err, tt.wantErr) {
				t.Errorf("Err = %v, want %v", r.Err, tt.wantErr)
			}
			if !tt.ok && r.Err == nil {
				t.Error("Err = nil for a failed check")
			}
			if !slices.Equal(g.requested, tt.requested) {
				t.Errorf("requested %v, want %v", g.requested, tt.requested)
			}
			checkResult(t, r, tt.host, tt.ok)
		})
	}
}

func checkResult(t *testing.T, r probe.Result, host string, ok bool) {
	t.Helper()
	if r.Host != host || r.Kind != probe.KindCard {
		t.Errorf("Host, Kind = %q, %q, want %q, %q", r.Host, r.Kind, host, probe.KindCard)
	}
	if r.At.IsZero() || r.Latency < 0 {
		t.Errorf("At = %v, Latency = %v", r.At, r.Latency)
	}
	if ok != (r.Card != nil) || ok != (r.Raw != nil) {
		t.Errorf("Card set = %v, Raw set = %v, want %v", r.Card != nil, r.Raw != nil, ok)
	}
}
