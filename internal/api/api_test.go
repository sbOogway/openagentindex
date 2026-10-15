package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sbOogway/openagentindex/internal/api"
	"github.com/sbOogway/openagentindex/internal/index"
)

const origin = "https://openagentindex.example"

var errDisk = errors.New("disk I/O error")

type fakeIndex struct {
	query     index.Query
	result    index.Results
	agent     index.Agent
	tags      []index.TagCount
	tagPrefix string
	tagLimit  int
	err       error
}

func (f *fakeIndex) Tags(_ context.Context, prefix string, limit int) ([]index.TagCount, error) {
	f.tagPrefix, f.tagLimit = prefix, limit
	return f.tags, f.err
}

func (f *fakeIndex) Search(_ context.Context, q index.Query) (index.Results, error) {
	f.query = q
	return f.result, f.err
}

func (f *fakeIndex) Agent(_ context.Context, host string) (index.Agent, error) {
	if f.err != nil {
		return index.Agent{}, f.err
	}
	if host != f.agent.Host {
		return index.Agent{}, fmt.Errorf("%w: %s", index.ErrNotFound, host)
	}
	return f.agent, nil
}

var gamma = index.Summary{
	Host: "a.example", Name: "Gamma", Description: "Echoes", ProtocolVersion: "1.0",
	OK: true, HTTPStatus: 200, LastChecked: time.Unix(1792000000, 0),
	Tags: []string{"echo", "test"}, Bindings: []string{"JSONRPC"},
}

func serve(t *testing.T, ix *fakeIndex, cors, target string) *httptest.ResponseRecorder {
	t.Helper()
	h := api.Handler(api.Config{Index: ix, CORSOrigin: cors, Log: slog.New(slog.DiscardHandler)})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	return w
}

func assertOK(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var got, exp any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body %q: %v", w.Body, err)
	}
	if err := json.Unmarshal([]byte(want), &exp); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, exp) {
		t.Errorf("body = %s, want %s", w.Body, want)
	}
}

func TestSearch(t *testing.T) {
	ix := &fakeIndex{result: index.Results{Total: 7, Agents: []index.Summary{gamma}}}
	w := serve(t, ix, "", "/v1/search")
	assertOK(t, w, `{"total": 7, "results": [{
		"host": "a.example", "name": "Gamma", "description": "Echoes", "protocol_version": "1.0",
		"status": "ok", "http_status": 200, "last_checked": 1792000000,
		"tags": ["echo", "test"], "bindings": ["JSONRPC"]}]}`)
	if want := (index.Query{Status: index.StatusOK, Limit: 20}); !reflect.DeepEqual(ix.query, want) {
		t.Errorf("query = %+v, want %+v", ix.query, want)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("Cache-Control = %q", cc)
	}
}

func TestSearchParameters(t *testing.T) {
	ix := &fakeIndex{}
	w := serve(t, ix, "", "/v1/search?q=echo+bot&status=failed&limit=5&offset=10&tag=test&tag=echo&binding=GRPC&version=0.3")
	assertOK(t, w, `{"total": 0, "results": []}`)
	want := index.Query{
		Text: "echo bot", Status: index.StatusFailed, Limit: 5, Offset: 10,
		Tags: []string{"test", "echo"}, Binding: "GRPC", Version: "0.3",
	}
	if !reflect.DeepEqual(ix.query, want) {
		t.Errorf("query = %+v, want %+v", ix.query, want)
	}
}

func TestSearchErrors(t *testing.T) {
	tests := []struct {
		name   string
		target string
		err    error
		status int
	}{
		{name: "limit not a number", target: "/v1/search?limit=x", status: http.StatusBadRequest},
		{name: "offset not a number", target: "/v1/search?offset=x", status: http.StatusBadRequest},
		{name: "invalid query", target: "/v1/search", err: fmt.Errorf("%w: limit 0", index.ErrInvalidQuery), status: http.StatusBadRequest},
		{name: "index error", target: "/v1/search", err: errDisk, status: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(t, &fakeIndex{err: tt.err}, "", tt.target)
			var body struct{ Error string }
			if w.Code != tt.status || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error == "" {
				t.Errorf("response = %d %s, want %d with an error message", w.Code, w.Body, tt.status)
			}
			if strings.Contains(body.Error, errDisk.Error()) {
				t.Errorf("error message %q shows the internal error", body.Error)
			}
		})
	}
}

func TestAgent(t *testing.T) {
	ix := &fakeIndex{agent: index.Agent{
		Summary: gamma,
		Card:    json.RawMessage(`{"name":"Gamma"}`),
		Observations: []index.Observation{{
			ID: "0191", Kind: index.KindCard, At: time.Unix(1792000000, 0), OK: false,
			HTTPStatus: 503, Latency: 250 * time.Millisecond, Error: "card request: status 503",
		}},
	}}
	w := serve(t, ix, "", "/v1/agents/a.example")
	assertOK(t, w, `{
		"host": "a.example", "name": "Gamma", "description": "Echoes", "protocol_version": "1.0",
		"status": "ok", "http_status": 200, "last_checked": 1792000000,
		"tags": ["echo", "test"], "bindings": ["JSONRPC"],
		"card": {"name": "Gamma"},
		"observations": [{"id": "0191", "kind": "card", "at": 1792000000, "ok": false,
			"http_status": 503, "latency_ms": 250, "error": "card request: status 503"}]}`)
}

func TestAgentWithoutCard(t *testing.T) {
	ix := &fakeIndex{agent: index.Agent{Summary: index.Summary{Host: "dead.example", HTTPStatus: 404, Error: "status 404", LastChecked: time.Unix(5, 0)}}}
	w := serve(t, ix, "", "/v1/agents/dead.example")
	assertOK(t, w, `{
		"host": "dead.example", "name": "", "description": "", "protocol_version": "",
		"status": "failed", "http_status": 404, "error": "status 404", "last_checked": 5,
		"tags": [], "bindings": [], "card": null, "observations": []}`)
}

func TestNotFound(t *testing.T) {
	for _, target := range []string{"/v1/agents/unknown.example", "/v2/search", "/"} {
		w := serve(t, &fakeIndex{agent: index.Agent{Summary: gamma}}, "", target)
		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", target, w.Code)
		}
	}
}

func TestHealthz(t *testing.T) {
	assertOK(t, serve(t, &fakeIndex{}, "", "/healthz"), `{"status": "ok"}`)
}

func TestCORS(t *testing.T) {
	tests := []struct {
		name string
		cors string
	}{
		{name: "origin set", cors: origin},
		{name: "no origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(t, &fakeIndex{}, tt.cors, "/v1/search")
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tt.cors {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tt.cors)
			}
		})
	}
}

func TestTags(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantPrefix string
		wantLimit  int
	}{
		{name: "defaults", target: "/v1/tags", wantLimit: 50},
		{name: "prefix and limit", target: "/v1/tags?q=we&limit=5", wantPrefix: "we", wantLimit: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := &fakeIndex{tags: []index.TagCount{{Tag: "weather", Hosts: 2}}}
			w := serve(t, ix, "", tt.target)
			assertOK(t, w, `{"tags": [{"tag": "weather", "hosts": 2}]}`)
			if ix.tagPrefix != tt.wantPrefix || ix.tagLimit != tt.wantLimit {
				t.Errorf("prefix, limit = %q, %d, want %q, %d", ix.tagPrefix, ix.tagLimit, tt.wantPrefix, tt.wantLimit)
			}
			if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=60" {
				t.Errorf("Cache-Control = %q", cc)
			}
		})
	}
}

func TestTagsEmpty(t *testing.T) {
	assertOK(t, serve(t, &fakeIndex{}, "", "/v1/tags"), `{"tags": []}`)
}

func TestTagsErrors(t *testing.T) {
	tests := []struct {
		name   string
		target string
		err    error
	}{
		{name: "limit not a number", target: "/v1/tags?limit=x"},
		{name: "invalid limit", target: "/v1/tags?limit=0", err: fmt.Errorf("%w: limit 0", index.ErrInvalidQuery)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if w := serve(t, &fakeIndex{err: tt.err}, "", tt.target); w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}
