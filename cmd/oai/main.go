package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sbOogway/openagentindex/internal/chat"
	"github.com/sbOogway/openagentindex/internal/config"
	"github.com/sbOogway/openagentindex/internal/fetch"
	"github.com/sbOogway/openagentindex/internal/index"
	"github.com/sbOogway/openagentindex/internal/tui"
)

// chatTimeout limits one chat request. Agents that use a language model can be slow.
const chatTimeout = 2 * time.Minute

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
	return tui.Run(ctx, tui.Config{Index: idx, Chat: chat.New(fetch.HTTPClient(chatTimeout))})
}
