package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sbOogway/openagentindex/internal/api"
	"github.com/sbOogway/openagentindex/internal/config"
	"github.com/sbOogway/openagentindex/internal/index"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("api failed", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadAPI()
	if err != nil {
		return err
	}
	idx, err := index.OpenReadOnly(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer idx.Close()

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.Handler(api.Config{Index: idx, CORSOrigin: cfg.CORSOrigin, Log: log}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	return serve(ctx, srv, log)
}

func serve(ctx context.Context, srv *http.Server, log *slog.Logger) error {
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("api started", "addr", srv.Addr)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("api stopped")
	return nil
}
