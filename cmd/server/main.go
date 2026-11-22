// Command server runs the GoNews API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gonews/gonews/internal/app"
	"github.com/gonews/gonews/internal/config"
	"github.com/gonews/gonews/internal/version"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "error", err)
		return 1
	}

	application, err := app.New(cfg)
	if err != nil {
		slog.Error("failed to initialize application", "error", err)
		return 1
	}
	defer application.Close()
	log := application.Logger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application.Start(ctx)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("GoNews API started",
			"version", version.Info(),
			"addr", cfg.HTTPAddr,
			"env", cfg.Env,
			"docs", cfg.PublicBaseURL+"/docs",
			"db", cfg.DatabasePath,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		log.Error("http server failed", "error", err)
		return 1
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		_ = srv.Close()
		return 1
	}
	log.Info("server stopped")
	return 0
}
