package probe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/sbOogway/openagentindex/internal/card"
)

const KindCard = "card"

var ErrBadHost = errors.New("host is not a plain host[:port]")

var cardPaths = []string{"/.well-known/agent-card.json", "/.well-known/agent.json"}

type Getter interface {
	Get(ctx context.Context, url string) (int, []byte, error)
}

type Prober struct {
	get Getter
}

func New(g Getter) *Prober {
	return &Prober{get: g}
}

type Result struct {
	Host       string
	Kind       string
	At         time.Time
	OK         bool
	HTTPStatus int
	Latency    time.Duration
	Err        error
	Card       *a2a.AgentCard
	Raw        []byte
}

// CheckCard gets the agent card of host. If the first card path gives 404, it tries the v0.3 path.
func (p *Prober) CheckCard(ctx context.Context, host string) (r Result) {
	r = Result{Host: host, Kind: KindCard, At: time.Now()}
	defer func() { r.Latency = time.Since(r.At) }()

	if !isPlainHost(host) {
		r.Err = fmt.Errorf("%w: %q", ErrBadHost, host)
		return r
	}
	status, body, err := p.getCard(ctx, host)
	r.HTTPStatus = status
	if err != nil {
		r.Err = err
		return r
	}
	c, err := card.Parse(body)
	if err != nil {
		r.Err = err
		return r
	}
	r.OK, r.Card, r.Raw = true, c, body
	return r
}

func (p *Prober) getCard(ctx context.Context, host string) (int, []byte, error) {
	var status int
	var body []byte
	for _, path := range cardPaths {
		var err error
		status, body, err = p.get.Get(ctx, cardURL(host, path))
		if err != nil {
			return status, nil, err
		}
		if status != http.StatusNotFound {
			break
		}
	}
	if status != http.StatusOK {
		return status, nil, fmt.Errorf("card request: status %d", status)
	}
	return status, body, nil
}

func cardURL(host, path string) string {
	u := url.URL{Scheme: "https", Host: host, Path: path}
	return u.String()
}

func isPlainHost(host string) bool {
	u, err := url.Parse("https://" + host)
	return err == nil && host != "" && u.Host == host
}
