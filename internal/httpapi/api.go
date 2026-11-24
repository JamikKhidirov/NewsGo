// Package httpapi exposes the public REST API.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gonews/gonews/internal/auth"
	"github.com/gonews/gonews/internal/cache"
	"github.com/gonews/gonews/internal/config"
	"github.com/gonews/gonews/internal/events"
	"github.com/gonews/gonews/internal/news"
	"github.com/gonews/gonews/internal/push"
	"github.com/gonews/gonews/internal/store"
	"github.com/gonews/gonews/internal/translate"
	"github.com/gonews/gonews/internal/version"
)

// API holds all HTTP dependencies.
type API struct {
	cfg         *config.Config
	st          *store.Store
	auth        *auth.Service
	agg         *news.Aggregator
	translator  *translate.Worker
	push        *push.Service
	cache       *cache.Cache
	bus         events.Bus
	log         *slog.Logger
	metrics     *Metrics
	startedAt   time.Time
	limiter     *rateLimiter
	authLimiter *rateLimiter
}

// New wires the HTTP API.
func New(cfg *config.Config, st *store.Store, authSvc *auth.Service, agg *news.Aggregator, tw *translate.Worker, ps *push.Service, cacheLayer *cache.Cache, bus events.Bus, log *slog.Logger) *API {
	if log == nil {
		log = slog.Default()
	}
	return &API{
		cfg:         cfg,
		st:          st,
		auth:        authSvc,
		agg:         agg,
		translator:  tw,
		push:        ps,
		cache:       cacheLayer,
		bus:         bus,
		log:         log,
		metrics:     &Metrics{StartedAt: time.Now().UTC()},
		startedAt:   time.Now().UTC(),
		limiter:     newRateLimiter(cfg.RateLimitPerMinute, time.Minute),
		authLimiter: newRateLimiter(20, time.Minute),
	}
}

// Router builds the chi router with all routes and middleware.
func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(a.recoverer)
	r.Use(a.requestID)
	r.Use(a.accessLog)
	r.Use(a.cors)
	r.Use(a.bodyLimit)
	r.Use(a.rateLimitMiddleware)
	r.Use(a.cacheMiddleware)

	r.Get("/", a.handleRoot)
	r.Get("/healthz", a.handleHealth)
	r.Get("/readyz", a.handleReady)
	r.Get("/metrics", a.handleMetrics)
	r.Get("/docs", a.handleGuide)
	r.Get("/docs/", a.handleGuide)
	r.Get("/docs/swagger", a.handleSwaggerUI)
	r.Get("/docs/openapi.yaml", a.handleOpenAPI)
	r.Get("/docs/postman_collection.json", a.handlePostmanCollection)
	r.Get("/docs/postman_environment.json", a.handlePostmanEnvironment)
	r.Get("/docs/env.example", a.handleEnvExample)
	r.Get("/docs/LICENSE", a.handleLicense)
	r.Get("/docs/assets/{file}", a.handleDocsAsset)
	r.Get("/docs/{page}", a.handleGuide)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", a.handleHealth)
		r.Get("/categories", a.handleCategories)
		r.Get("/sources", a.handleSources)
		r.Get("/tags", a.handleTags)
		r.Get("/tags/{slug}", a.handleTag)

		r.With(a.optionalAuth).Get("/news", a.handleListNews)
		r.With(a.optionalAuth).Get("/news/trending", a.handleTrending)
		r.With(a.optionalAuth).Get("/news/{id}", a.handleGetNews)
		r.Get("/news/{id}/comments", a.handleListComments)
		r.With(a.optionalAuth).Get("/search", a.handleSearch)

		r.With(a.authRateLimit).Post("/auth/register", a.handleRegister)
		r.With(a.authRateLimit).Post("/auth/login", a.handleLogin)
		r.With(a.authRateLimit).Post("/auth/refresh", a.handleRefresh)
		r.Post("/auth/logout", a.handleLogout)

		r.Group(func(r chi.Router) {
			r.Use(a.requireAuth)

			r.Get("/me", a.handleMe)
			r.Patch("/me", a.handleUpdateMe)
			r.Post("/me/password", a.handleChangePassword)
			r.Get("/me/stats", a.handleMeStats)
			r.Get("/me/feed", a.handleFeed)
			r.Get("/me/bookmarks", a.handleBookmarks)
			r.Get("/me/likes", a.handleLikes)
			r.Get("/me/history", a.handleHistory)
			r.Get("/me/follows", a.handleListFollows)
			r.Put("/me/follows/{slug}", a.handleFollow)
			r.Delete("/me/follows/{slug}", a.handleUnfollow)
			r.Get("/me/devices", a.handleListDevices)
			r.Post("/me/devices", a.handleRegisterDevice)
			r.Delete("/me/devices/{token}", a.handleDeleteDevice)
			r.Get("/me/notifications", a.handleListNotifications)
			r.Get("/me/notifications/unread-count", a.handleUnreadCount)
			r.Post("/me/notifications/{id}/read", a.handleMarkRead)
			r.Post("/me/notifications/read-all", a.handleMarkAllRead)

			r.Post("/news/{id}/like", a.handleLike)
			r.Delete("/news/{id}/like", a.handleUnlike)
			r.Post("/news/{id}/bookmark", a.handleBookmark)
			r.Delete("/news/{id}/bookmark", a.handleUnbookmark)
			r.Post("/news/{id}/view", a.handleView)
			r.Post("/news/{id}/comments", a.handleCreateComment)
			r.Patch("/comments/{id}", a.handleUpdateComment)
			r.Delete("/comments/{id}", a.handleDeleteComment)
		})

		r.Group(func(r chi.Router) {
			r.Use(a.requireAdmin)
			r.Post("/admin/ingest", a.handleAdminIngest)
			r.Get("/admin/ingest-runs", a.handleAdminIngestRuns)
			r.Post("/admin/translate/backfill", a.handleAdminBackfill)
			r.Get("/admin/translation-stats", a.handleAdminTranslationStats)
			r.Post("/admin/push", a.handleAdminPush)
			r.Get("/admin/stats", a.handleAdminStats)
			r.Post("/admin/purge", a.handleAdminPurge)
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})
	return r
}

func (a *API) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":      "GoNews API",
		"version":   version.Version,
		"build":     version.Info(),
		"docs":      a.cfg.PublicBaseURL + "/docs",
		"swagger":   a.cfg.PublicBaseURL + "/docs/swagger",
		"openapi":   a.cfg.PublicBaseURL + "/docs/openapi.yaml",
		"postman":   a.cfg.PublicBaseURL + "/docs/postman_collection.json",
		"health":    a.cfg.PublicBaseURL + "/healthz",
		"languages": []string{"ru", "en"},
		"time":      time.Now().UTC(),
	})
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.st.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "database is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": version.Version,
		"time":    time.Now().UTC(),
	})
}

func (a *API) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	components := map[string]string{"postgres": "ok", "redis": "disabled", "kafka": "local"}
	ready := true
	if err := a.st.Ping(ctx); err != nil {
		components["postgres"] = "error: " + err.Error()
		ready = false
	}
	if a.cache != nil && a.cache.Enabled() {
		if err := a.cache.Healthy(ctx); err != nil {
			components["redis"] = "error: " + err.Error()
			ready = false
		} else {
			components["redis"] = "ok"
		}
	}
	if a.bus != nil {
		components["kafka"] = a.bus.Name()
		if err := a.bus.Healthy(ctx); err != nil {
			components["kafka"] = "error: " + err.Error()
			ready = false
		}
	}
	status := "ready"
	code := http.StatusOK
	if !ready {
		status = "not_ready"
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{"status": status, "components": components})
}

// --- helpers ---

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

func writeData(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, dataResponse{Data: v})
}

func writeList(w http.ResponseWriter, status int, data any, meta metaDTO) {
	writeJSON(w, status, listResponse{Data: data, Meta: meta})
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func normalizePage(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return page, limit
}

func queryInt(r *http.Request, key string, def int) int {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
