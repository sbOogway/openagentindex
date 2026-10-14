package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sbOogway/openagentindex/internal/crawler"
	"github.com/sbOogway/openagentindex/internal/fetch"
	"github.com/sbOogway/openagentindex/internal/index"
	"github.com/sbOogway/openagentindex/internal/leads"
	"github.com/sbOogway/openagentindex/internal/probe"
)

var version = "dev"

const defaultContactURL = "https://github.com/sbOogway/openagentindex"

type config struct {
	dbPath     string
	seedsPath  string
	contactURL string
}

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

	cfg, err := readConfig()
	if err != nil {
		return err
	}
	idx, err := index.Open(ctx, cfg.dbPath)
	if err != nil {
		return err
	}
	defer idx.Close()

	userAgent := fmt.Sprintf("OpenAgentIndexBot/%s (+%s)", version, cfg.contactURL)
	c := crawler.New(crawler.Config{
		Sources: []crawler.Source{leads.SeedFile{Path: cfg.seedsPath}},
		Prober:  probe.New(fetch.New(userAgent)),
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

func readConfig() (config, error) {
	cfg := config{
		dbPath:     os.Getenv("OAI_DB_PATH"),
		seedsPath:  os.Getenv("OAI_SEEDS_PATH"),
		contactURL: os.Getenv("OAI_CONTACT_URL"),
	}
	if cfg.dbPath == "" || cfg.seedsPath == "" {
		return config{}, fmt.Errorf("set OAI_DB_PATH and OAI_SEEDS_PATH")
	}
	if cfg.contactURL == "" {
		cfg.contactURL = defaultContactURL
	}
	return cfg, nil
}
