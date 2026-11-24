package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gonews/gonews/internal/auth"
	"github.com/gonews/gonews/internal/config"
	"github.com/gonews/gonews/internal/httpapi"
	"github.com/gonews/gonews/internal/news"
	"github.com/gonews/gonews/internal/push"
	"github.com/gonews/gonews/internal/testutil"
	"github.com/gonews/gonews/internal/translate"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := testutil.OpenStore(t)

	cfg := &config.Config{
		Env:                "test",
		HTTPAddr:           ":0",
		PublicBaseURL:      "http://localhost:8080",
		JWTSecret:          "test-secret",
		AccessTTL:          time.Minute,
		RefreshTTL:         time.Hour,
		AdminToken:         "test-admin-token",
		CORSOrigins:        []string{"*"},
		RateLimitPerMinute: 10000,
		IngestTimeout:      time.Second,
		PushInterval:       time.Minute,
	}
	authSvc := auth.NewService(st, cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL, nil)
	agg := news.NewAggregator(st, nil, []string{"ru"}, 10, log)
	if err := agg.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	translator := translate.New(nil, "", "", time.Second, 1000, log)
	worker := translate.NewWorker(st, translator, 1, log)
	delivery, err := push.NewDelivery("", "", "", "", "", "", false, log)
	if err != nil {
		t.Fatal(err)
	}
	pushSvc := push.NewService(st, nil, delivery, false, "gonews.notifications", log)
	return httpapi.New(cfg, st, authSvc, agg, worker, pushSvc, nil, nil, log).Router()
}

func request(t *testing.T, h http.Handler, method, path, body, token, adminToken string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if adminToken != "" {
		req.Header.Set("X-Admin-Token", adminToken)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPublicAndAuthFlow(t *testing.T) {
	h := newTestHandler(t)

	if rec := request(t, h, http.MethodGet, "/healthz", "", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("healthz: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request(t, h, http.MethodGet, "/api/v1/news?lang=ru", "", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("news: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request(t, h, http.MethodGet, "/api/v1/me", "", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me must require auth, got %d", rec.Code)
	}

	reg := request(t, h, http.MethodPost, "/api/v1/auth/register", `{"email":"reader@example.com","password":"password123","displayName":"Reader"}`, "", "")
	if reg.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", reg.Code, reg.Body.String())
	}
	var envelope struct {
		Data struct {
			AccessToken string `json:"accessToken"`
			User        struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(reg.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.AccessToken == "" {
		t.Fatal("access token missing")
	}

	me := request(t, h, http.MethodGet, "/api/v1/me", "", envelope.Data.AccessToken, "")
	if me.Code != http.StatusOK {
		t.Fatalf("me: %d %s", me.Code, me.Body.String())
	}

	comment := request(t, h, http.MethodPost, "/api/v1/news/missing/comments", `{"body":"hello"}`, envelope.Data.AccessToken, "")
	if comment.Code != http.StatusNotFound {
		t.Fatalf("comment on missing article: %d %s", comment.Code, comment.Body.String())
	}

	tags := request(t, h, http.MethodGet, "/api/v1/tags?lang=ru", "", "", "")
	if tags.Code != http.StatusOK {
		t.Fatalf("tags: %d %s", tags.Code, tags.Body.String())
	}

	admin := request(t, h, http.MethodGet, "/api/v1/admin/stats", "", "", "test-admin-token")
	if admin.Code != http.StatusOK {
		t.Fatalf("admin stats: %d %s", admin.Code, admin.Body.String())
	}
	if rec := request(t, h, http.MethodGet, "/api/v1/admin/stats", "", "", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("admin stats must reject a bad token, got %d", rec.Code)
	}

	if rec := request(t, h, http.MethodGet, "/docs", "", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("guide: %d", rec.Code)
	} else {
		body := rec.Body.String()
		for _, want := range []string{"data-theme-toggle", "/docs/assets/guide.css", "Обзор и быстрый старт"} {
			if !strings.Contains(body, want) {
				t.Fatalf("guide is missing %q", want)
			}
		}
	}
	if rec := request(t, h, http.MethodGet, "/docs/api", "", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("guide page: %d", rec.Code)
	}
	if rec := request(t, h, http.MethodGet, "/docs/swagger", "", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("swagger: %d", rec.Code)
	} else if !strings.Contains(rec.Body.String(), "/docs/assets/swagger-ui-bundle.js") {
		t.Fatal("swagger must use the embedded bundle")
	}
	if rec := request(t, h, http.MethodGet, "/docs/assets/guide.css", "", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("guide css: %d", rec.Code)
	}
	if rec := request(t, h, http.MethodGet, "/docs/postman_collection.json", "", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("postman collection: %d", rec.Code)
	}
	if rec := request(t, h, http.MethodGet, "/docs/env.example", "", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("env example: %d", rec.Code)
	}
	if rec := request(t, h, http.MethodGet, "/docs/does-not-exist", "", "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown guide page must be 404, got %d", rec.Code)
	}
	if rec := request(t, h, http.MethodGet, "/docs/openapi.yaml", "", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("openapi: %d", rec.Code)
	}
}

func TestLoginValidation(t *testing.T) {
	h := newTestHandler(t)
	reg := request(t, h, http.MethodPost, "/api/v1/auth/register", `{"email":"login@example.com","password":"password123"}`, "", "")
	if reg.Code != http.StatusCreated {
		t.Fatalf("register: %d", reg.Code)
	}
	bad := request(t, h, http.MethodPost, "/api/v1/auth/login", `{"email":"login@example.com","password":"nope"}`, "", "")
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad login must be 401, got %d", bad.Code)
	}
	dup := request(t, h, http.MethodPost, "/api/v1/auth/register", `{"email":"login@example.com","password":"password123"}`, "", "")
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate email must be 409, got %d", dup.Code)
	}
}
