package leads_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/sbOogway/openagentindex/internal/leads"
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

func pageURL(offset int) string {
	return fmt.Sprintf("https://a2aregistry.org/api/agents?limit=50&offset=%d", offset)
}

func page(total int, uris ...string) response {
	body := `{"agents":[`
	for i, u := range uris {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"name":"x","wellKnownURI":%q}`, u)
	}
	body += fmt.Sprintf(`],"total":%d,"limit":50,"offset":0}`, total)
	return response{status: 200, body: body}
}

func uris(first, n int) []string {
	var out []string
	for i := first; i < first+n; i++ {
		out = append(out, fmt.Sprintf("https://a%d.example/.well-known/agent-card.json", i))
	}
	return out
}

func hosts(first, n int) []string {
	var out []string
	for i := first; i < first+n; i++ {
		out = append(out, fmt.Sprintf("a%d.example", i))
	}
	return out
}

func TestA2ARegistryHosts(t *testing.T) {
	tests := []struct {
		name      string
		responses map[string]response
		want      []string
		requested []string
	}{
		{
			name:      "one page",
			responses: map[string]response{pageURL(0): page(2, uris(0, 2)...)},
			want:      hosts(0, 2),
			requested: []string{pageURL(0)},
		},
		{
			name: "pages until total",
			responses: map[string]response{
				pageURL(0):   page(120, uris(0, 50)...),
				pageURL(50):  page(120, uris(50, 50)...),
				pageURL(100): page(120, uris(100, 20)...),
			},
			want:      hosts(0, 120),
			requested: []string{pageURL(0), pageURL(50), pageURL(100)},
		},
		{
			name: "empty page before total",
			responses: map[string]response{
				pageURL(0):  page(80, uris(0, 50)...),
				pageURL(50): page(80),
			},
			want:      hosts(0, 50),
			requested: []string{pageURL(0), pageURL(50)},
		},
		{
			name:      "empty registry",
			responses: map[string]response{pageURL(0): page(0)},
			requested: []string{pageURL(0)},
		},
		{
			name: "host of each https URI",
			responses: map[string]response{pageURL(0): page(7,
				"https://A.Example/.well-known/agent-card.json",
				"https://b.example:8443/.well-known/agent.json",
				"https://c.example/agents/x/agent-card.json",
				"http://d.example:8082/.well-known/agent-card.json",
				"",
				"::not a url",
				"/.well-known/agent-card.json",
			)},
			want:      []string{"a.example", "b.example:8443", "c.example"},
			requested: []string{pageURL(0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &fakeGetter{responses: tt.responses}
			got, err := leads.NewA2ARegistry(g).Hosts(context.Background())
			if err != nil {
				t.Fatalf("Hosts: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Hosts = %q, want %q", got, tt.want)
			}
			if !slices.Equal(g.requested, tt.requested) {
				t.Errorf("requested = %q, want %q", g.requested, tt.requested)
			}
		})
	}
}

func TestA2ARegistryErrors(t *testing.T) {
	tests := []struct {
		name     string
		response response
		want     error
	}{
		{name: "request error", response: response{err: errNetwork}, want: errNetwork},
		{name: "status 503", response: response{status: 503}},
		{name: "not JSON", response: response{status: 200, body: "<html>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &fakeGetter{responses: map[string]response{
				pageURL(0):  page(60, uris(0, 50)...),
				pageURL(50): tt.response,
			}}
			got, err := leads.NewA2ARegistry(g).Hosts(context.Background())
			if err == nil {
				t.Fatalf("Hosts = %q, want an error", got)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
