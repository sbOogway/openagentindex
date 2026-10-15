package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sbOogway/openagentindex/internal/config"
	"github.com/sbOogway/openagentindex/internal/index"
	"github.com/sbOogway/openagentindex/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "oai:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadTUI()
	if err != nil {
		return err
	}
	if _, err := os.Stat(cfg.DBPath); err != nil {
		return fmt.Errorf("no index at %s (set db_path in [tui] of oai.toml or OAI_DB_PATH; make run-crawler creates it): %w", cfg.DBPath, err)
	}
	idx, err := index.OpenReadOnly(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer idx.Close()
	return tui.Run(ctx, tui.Config{Index: idx})
}
