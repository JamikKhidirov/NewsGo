package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gonews/gonews/internal/langdetect"
	"github.com/gonews/gonews/internal/news"
)

func (a *API) handleAdminIngest(w http.ResponseWriter, r *http.Request) {
	// Let the run finish even if the admin client disconnects.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), a.cfg.IngestTimeout+60*time.Second)
	defer cancel()
	stats, err := a.agg.Run(ctx)
	if err != nil {
		if errors.Is(err, news.ErrAlreadyRunning) {
			writeError(w, http.StatusConflict, "ingest_running", "ingestion is already running")
			return
		}
		a.log.Error("admin ingest", "error", err)
		writeError(w, http.StatusInternalServerError, "ingest_failed", "ingestion failed")
		return
	}
	a.metrics.IngestRun()
	if a.translator != nil {
		a.translator.Notify()
	}
	if a.cfg.PushBroadcast {
		go func() {
			bctx, bcancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer bcancel()
			if _, sent, err := a.push.BroadcastTopNews(bctx, a.cfg.PushMinScore, a.cfg.PushInterval); err != nil {
				a.log.Warn("broadcast top news", "error", err)
			} else if sent {
				a.metrics.Push()
			}
		}()
	}
	writeData(w, http.StatusOK, stats)
}

func (a *API) handleAdminIngestRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := a.st.ListIngestRuns(r.Context(), queryInt(r, "limit", 20))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, runs)
}

type backfillRequest struct {
	Lang  string `json:"lang"`
	Limit int    `json:"limit"`
}

func (a *API) handleAdminBackfill(w http.ResponseWriter, r *http.Request) {
	var req backfillRequest
	_ = decodeJSON(r, &req)
	lang := langdetect.Normalize(req.Lang)
	if len(lang) != 2 {
		lang = "ru"
	}
	if req.Limit <= 0 || req.Limit > 5000 {
		req.Limit = 500
	}
	queued, err := a.st.BackfillTranslations(r.Context(), lang, req.Limit)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if a.translator != nil {
		a.translator.Notify()
	}
	writeData(w, http.StatusOK, map[string]any{"queued": queued, "lang": lang})
}

func (a *API) handleAdminTranslationStats(w http.ResponseWriter, r *http.Request) {
	pending, failed, done, err := a.st.TranslationStats(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]int{"pending": pending, "failed": failed, "done": done})
}

type adminPushRequest struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	ArticleID string `json:"articleId"`
	UserID    string `json:"userId"`
}

func (a *API) handleAdminPush(w http.ResponseWriter, r *http.Request) {
	var req adminPushRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	if n := len([]rune(req.Title)); n < 1 || n > 120 {
		writeError(w, http.StatusBadRequest, "validation_error", "title must contain 1-120 characters")
		return
	}
	if len([]rune(req.Body)) > 500 {
		writeError(w, http.StatusBadRequest, "validation_error", "body must not exceed 500 characters")
		return
	}
	if req.ArticleID != "" {
		if _, err := a.st.GetArticleRaw(r.Context(), req.ArticleID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_article", "article not found")
			return
		}
	}
	var (
		res any
		err error
	)
	if req.UserID != "" {
		res, err = a.push.SendToUser(r.Context(), req.UserID, req.Title, req.Body, req.ArticleID, "admin")
	} else {
		res, err = a.push.Broadcast(r.Context(), req.Title, req.Body, req.ArticleID, "admin")
	}
	if err != nil {
		a.log.Error("admin push", "error", err)
		writeError(w, http.StatusBadGateway, "push_failed", "push delivery failed")
		return
	}
	a.metrics.Push()
	writeData(w, http.StatusOK, res)
}

func (a *API) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	articles, _ := a.st.CountArticles(r.Context())
	users, _ := a.st.CountUsers(r.Context())
	comments, _ := a.st.CountComments(r.Context())
	tags, _ := a.st.CountTags(r.Context())
	pending, failed, done, _ := a.st.TranslationStats(r.Context())
	runs, _ := a.st.ListIngestRuns(r.Context(), 1)
	var lastRun any
	if len(runs) > 0 {
		lastRun = runs[0]
	}
	writeData(w, http.StatusOK, map[string]any{
		"articles":      articles,
		"users":         users,
		"comments":      comments,
		"tags":          tags,
		"translation":   map[string]int{"pending": pending, "failed": failed, "done": done},
		"lastIngest":    lastRun,
		"uptimeSeconds": int(time.Since(a.startedAt).Seconds()),
		"version":       "see /",
	})
}

type purgeRequest struct {
	BeforeDays int `json:"beforeDays"`
}

func (a *API) handleAdminPurge(w http.ResponseWriter, r *http.Request) {
	var req purgeRequest
	_ = decodeJSON(r, &req)
	if req.BeforeDays <= 0 {
		req.BeforeDays = 90
	}
	if req.BeforeDays < 7 {
		writeError(w, http.StatusBadRequest, "validation_error", "beforeDays must be at least 7")
		return
	}
	deleted, err := a.st.PurgeOlderThan(r.Context(), time.Now().UTC().AddDate(0, 0, -req.BeforeDays))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]int64{"deleted": deleted})
}
