// Package app wires configuration, storage, aggregation, translation, push and HTTP.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gonews/gonews/internal/auth"
	"github.com/gonews/gonews/internal/config"
	"github.com/gonews/gonews/internal/httpapi"
	"github.com/gonews/gonews/internal/news"
	"github.com/gonews/gonews/internal/push"
	"github.com/gonews/gonews/internal/store"
	"github.com/gonews/gonews/internal/translate"
)

// App owns all long-lived dependencies.
type App struct {
	cfg        *config.Config
	log        *slog.Logger
	st         *store.Store
	agg        *news.Aggregator
	translator *translate.Worker
	push       *push.Service
	api        *httpapi.API
}

// New builds the application.
func New(cfg *config.Config) (*App, error) {
	log := newLogger(cfg)
	ctx := context.Background()

	st, err := store.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	authSvc := auth.NewService(st, cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL, func(email string) string {
		if cfg.IsAdminEmail(email) {
			return "admin"
		}
		return "user"
	})

	httpClient := &http.Client{Timeout: 25 * time.Second}
	providers := news.BuildProviders(cfg.Providers, httpClient, cfg.UserAgent)
	agg := news.NewAggregator(st, providers, cfg.TranslateTargets, cfg.ProviderLimit, log)
	if err := agg.Seed(ctx); err != nil {
		st.Close()
		return nil, err
	}

	translateNames := cfg.TranslateProviders
	if !cfg.TranslateEnabled {
		translateNames = nil
	}
	translator := translate.New(translateNames, cfg.LibreTranslateURL, cfg.LingvaURL, cfg.TranslateTimeout, cfg.TranslateMaxChars, log)
	worker := translate.NewWorker(st, translator, cfg.TranslateWorkers, log)

	sender, err := push.NewSender(cfg.FCMProjectID, cfg.FCMServiceAccountFile, log)
	if err != nil {
		st.Close()
		return nil, err
	}
	pushSvc := push.NewService(st, sender, cfg.PushEnabled, log)

	api := httpapi.New(cfg, st, authSvc, agg, worker, pushSvc, log)
	return &App{
		cfg:        cfg,
		log:        log,
		st:         st,
		agg:        agg,
		translator: worker,
		push:       pushSvc,
		api:        api,
	}, nil
}

// Handler returns the HTTP handler.
func (a *App) Handler() http.Handler { return a.api.Router() }

// Logger exposes the structured logger.
func (a *App) Logger() *slog.Logger { return a.log }

// Start launches background workers. It returns immediately.
func (a *App) Start(ctx context.Context) {
	a.translator.Start(ctx)
	go a.scheduler(ctx)
}

// Close releases resources.
func (a *App) Close() error {
	return a.st.Close()
}

func (a *App) scheduler(ctx context.Context) {
	if a.cfg.IngestOnStart {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
		a.runIngest(ctx)
	}

	var ingestC <-chan time.Time
	if a.cfg.IngestInterval > 0 {
		ticker := time.NewTicker(a.cfg.IngestInterval)
		defer ticker.Stop()
		ingestC = ticker.C
	}

	retention := time.NewTicker(24 * time.Hour)
	defer retention.Stop()
	if a.cfg.RetentionDays > 0 {
		a.runRetention(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ingestC:
			a.runIngest(ctx)
		case <-retention.C:
			a.runRetention(ctx)
		}
	}
}

func (a *App) runIngest(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, a.cfg.IngestTimeout)
	defer cancel()
	stats, err := a.agg.Run(ctx)
	if err != nil {
		if !errors.Is(err, news.ErrAlreadyRunning) && !errors.Is(err, context.Canceled) {
			a.log.Error("scheduled ingestion failed", "error", err)
		}
		return
	}
	a.translator.Notify()
	if a.cfg.PushBroadcast {
		bctx, bcancel := context.WithTimeout(parent, 45*time.Second)
		defer bcancel()
		if _, sent, pErr := a.push.BroadcastTopNews(bctx, a.cfg.PushMinScore, a.cfg.PushInterval); pErr != nil {
			a.log.Warn("broadcast top news", "error", pErr)
		} else if sent {
			a.log.Info("breaking news push sent")
		}
	}
	a.log.Info("scheduled ingestion complete", "inserted", stats.Inserted, "updated", stats.Updated, "fetched", stats.Fetched)
}

func (a *App) runRetention(ctx context.Context) {
	if a.cfg.RetentionDays <= 0 {
		return
	}
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cutoff := time.Now().UTC().AddDate(0, 0, -a.cfg.RetentionDays)
	n, err := a.st.PurgeOlderThan(rctx, cutoff)
	if err != nil {
		a.log.Warn("retention purge failed", "error", err)
		return
	}
	if n > 0 {
		a.log.Info("retention purge", "deleted", n, "before_days", a.cfg.RetentionDays)
	}
}

func newLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
