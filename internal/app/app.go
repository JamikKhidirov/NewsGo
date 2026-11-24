// Package app wires configuration, storage, aggregation, translation, events,
// push delivery, metrics and HTTP.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gonews/gonews/internal/auth"
	"github.com/gonews/gonews/internal/cache"
	"github.com/gonews/gonews/internal/config"
	"github.com/gonews/gonews/internal/events"
	"github.com/gonews/gonews/internal/httpapi"
	"github.com/gonews/gonews/internal/metrics"
	"github.com/gonews/gonews/internal/news"
	"github.com/gonews/gonews/internal/push"
	"github.com/gonews/gonews/internal/store"
	"github.com/gonews/gonews/internal/translate"
	"github.com/prometheus/client_golang/prometheus"
)

// App owns all long-lived dependencies.
type App struct {
	cfg        *config.Config
	log        *slog.Logger
	st         *store.Store
	cache      *cache.Cache
	bus        events.Bus
	agg        *news.Aggregator
	translator *translate.Worker
	push       *push.Service
	pushWorker *push.Worker
	api        *httpapi.API
}

// New builds the application.
func New(cfg *config.Config) (*App, error) {
	log := newLogger(cfg)
	ctx := context.Background()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	// Database pool metrics.
	metrics.Registry.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "gonews_db_pool_open_connections",
			Help: "Open PostgreSQL connections.",
		}, func() float64 { return float64(st.DB().Stats().OpenConnections) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "gonews_db_pool_in_use",
			Help: "PostgreSQL connections currently in use.",
		}, func() float64 { return float64(st.DB().Stats().InUse) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "gonews_db_pool_wait_total",
			Help: "Total connection pool waits.",
		}, func() float64 { return float64(st.DB().Stats().WaitCount) }),
	)

	cacheLayer := cache.New(cfg.RedisURL, cfg.CacheTTL, cfg.CachePrefix, log)

	bus, err := events.New(cfg.KafkaBrokers, cfg.KafkaGroup, []string{cfg.KafkaPushTopic, cfg.KafkaNewsTopic}, log)
	if err != nil {
		st.Close()
		cacheLayer.Close()
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
		bus.Close()
		cacheLayer.Close()
		st.Close()
		return nil, err
	}

	translateNames := cfg.TranslateProviders
	if !cfg.TranslateEnabled {
		translateNames = nil
	}
	translator := translate.New(translateNames, cfg.LibreTranslateURL, cfg.LingvaURL, cfg.TranslateTimeout, cfg.TranslateMaxChars, log)
	worker := translate.NewWorker(st, translator, cfg.TranslateWorkers, log)

	delivery, err := push.NewDelivery(
		cfg.FCMProjectID, cfg.FCMServiceAccountFile,
		cfg.APNSKeyFile, cfg.APNSKeyID, cfg.APNSTeamID, cfg.APNSTopic, cfg.APNSSandbox, log,
	)
	if err != nil {
		bus.Close()
		cacheLayer.Close()
		st.Close()
		return nil, err
	}
	pushSvc := push.NewService(st, bus, delivery, cfg.PushEnabled, cfg.KafkaPushTopic, log)
	pushWorker := push.NewWorker(st, delivery, log)

	api := httpapi.New(cfg, st, authSvc, agg, worker, pushSvc, cacheLayer, bus, log)
	return &App{
		cfg:        cfg,
		log:        log,
		st:         st,
		cache:      cacheLayer,
		bus:        bus,
		agg:        agg,
		translator: worker,
		push:       pushSvc,
		pushWorker: pushWorker,
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
	a.pushWorker.Start(ctx, a.bus, a.cfg.KafkaPushTopic, a.cfg.KafkaGroup)
	go a.scheduler(ctx)
}

// Close releases resources.
func (a *App) Close() error {
	if a.bus != nil {
		a.bus.Close()
	}
	if a.cache != nil {
		_ = a.cache.Close()
	}
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
	a.cache.DeletePrefix(context.Background(), "http")
	metrics.IngestLastFinished.Set(float64(time.Now().Unix()))
	a.translator.Notify()
	a.publishIngestEvent(ctx, stats)
	if a.cfg.PushBroadcast {
		bctx, bcancel := context.WithTimeout(parent, 45*time.Second)
		defer bcancel()
		if _, sent, pErr := a.push.BroadcastTopNews(bctx, a.cfg.PushMinScore, a.cfg.PushInterval); pErr != nil {
			a.log.Warn("broadcast top news", "error", pErr)
		} else if sent {
			a.log.Info("breaking news push queued")
		}
	}
	a.log.Info("scheduled ingestion complete", "inserted", stats.Inserted, "updated", stats.Updated, "fetched", stats.Fetched)
}

// publishIngestEvent emits a compact news event for downstream consumers.
func (a *App) publishIngestEvent(ctx context.Context, stats *news.Stats) {
	if stats == nil {
		return
	}
	evt := map[string]any{
		"type":       "ingest.completed",
		"runId":      stats.RunID,
		"fetched":    stats.Fetched,
		"inserted":   stats.Inserted,
		"updated":    stats.Updated,
		"errors":     stats.Errors,
		"durationMs": stats.DurationMS,
		"finishedAt": stats.FinishedAt,
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}
	if err := a.bus.Publish(ctx, a.cfg.KafkaNewsTopic, "ingest", payload); err != nil {
		a.log.Debug("news event publish failed", "error", err)
	}
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
	if cfg.Env == "production" || strings.EqualFold(os.Getenv("GONEWS_LOG_FORMAT"), "json") {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
