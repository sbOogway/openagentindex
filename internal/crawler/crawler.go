package crawler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/sbOogway/openagentindex/internal/index"
	"github.com/sbOogway/openagentindex/internal/probe"
)

type Source interface {
	Hosts(ctx context.Context) ([]string, error)
}

type Prober interface {
	CheckCard(ctx context.Context, host string) probe.Result
}

type Store interface {
	Ingest(ctx context.Context, o index.Observation) error
}

type Config struct {
	Sources []Source
	Prober  Prober
	Store   Store
	Log     *slog.Logger
}

type Crawler struct {
	cfg Config
}

func New(cfg Config) *Crawler {
	return &Crawler{cfg: cfg}
}

// Run does one card check for each host of the sources and stores the results.
func (c *Crawler) Run(ctx context.Context) error {
	for _, host := range c.hosts(ctx) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.check(ctx, host); err != nil {
			return err
		}
	}
	return nil
}

func (c *Crawler) hosts(ctx context.Context) []string {
	var hosts []string
	seen := map[string]bool{}
	for _, s := range c.cfg.Sources {
		found, err := s.Hosts(ctx)
		if err != nil {
			c.cfg.Log.Error("lead source failed", "source", fmt.Sprintf("%T", s), "err", err)
			continue
		}
		for _, h := range found {
			if !seen[h] {
				seen[h] = true
				hosts = append(hosts, h)
			}
		}
	}
	return hosts
}

func (c *Crawler) check(ctx context.Context, host string) error {
	r := c.cfg.Prober.CheckCard(ctx, host)
	if errors.Is(r.Err, probe.ErrBadHost) {
		c.cfg.Log.Warn("skip lead", "host", host, "err", r.Err)
		return nil
	}
	o, err := observation(r)
	if err != nil {
		return err
	}
	if err := c.cfg.Store.Ingest(ctx, o); err != nil {
		return fmt.Errorf("ingest %s: %w", host, err)
	}
	c.logResult(r)
	return nil
}

func (c *Crawler) logResult(r probe.Result) {
	if r.OK {
		c.cfg.Log.Info("agent found", "host", r.Host, "name", r.Card.Name)
		return
	}
	c.cfg.Log.Info("no agent", "host", r.Host, "status", r.HTTPStatus, "err", r.Err)
}

func observation(r probe.Result) (index.Observation, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return index.Observation{}, err
	}
	o := index.Observation{
		ID:         id.String(),
		Host:       r.Host,
		Kind:       r.Kind,
		At:         r.At,
		OK:         r.OK,
		HTTPStatus: r.HTTPStatus,
		Latency:    r.Latency,
		Card:       r.Raw,
	}
	if r.Err != nil {
		o.Error = r.Err.Error()
	}
	return o, nil
}
