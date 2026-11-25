// Command server runs the GoNews API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			return healthcheck()
		case "version":
			fmt.Println(version.Info())
			return 0
		}
	}

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
			"database", "postgres",
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

// healthcheck is used by the distroless container HEALTHCHECK.
func healthcheck() int {
	addr := os.Getenv("GONEWS_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = "8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(strings.Trim(host, "[]"), port) + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
