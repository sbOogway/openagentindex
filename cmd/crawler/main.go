package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sbOogway/openagentindex/internal/config"
	"github.com/sbOogway/openagentindex/internal/crawler"
	"github.com/sbOogway/openagentindex/internal/fetch"
	"github.com/sbOogway/openagentindex/internal/index"
	"github.com/sbOogway/openagentindex/internal/leads"
	"github.com/sbOogway/openagentindex/internal/probe"
)

var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("crawler failed", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadCrawler()
	if err != nil {
		return err
	}
	idx, err := index.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer idx.Close()

	userAgent := fmt.Sprintf("OpenAgentIndexBot/%s (+%s)", version, cfg.ContactURL)
	client := fetch.New(userAgent)
	c := crawler.New(crawler.Config{
		Sources: []crawler.Source{leads.SeedFile{Path: cfg.SeedsPath}, leads.NewA2ARegistry(client)},
		Prober:  probe.New(client),
		Store:   idx,
		Log:     log,
	})
	log.Info("crawl started", "node", idx.NodeID())
	if err := c.Run(ctx); err != nil {
		return err
	}
	log.Info("crawl done")
	return nil
}
