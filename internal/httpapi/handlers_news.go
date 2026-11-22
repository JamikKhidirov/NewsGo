package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gonews/gonews/internal/langdetect"
	"github.com/gonews/gonews/internal/news"
	"github.com/gonews/gonews/internal/store"
)

func langParam(r *http.Request) string {
	raw := strings.TrimSpace(r.URL.Query().Get("lang"))
	if raw == "" {
		raw = r.Header.Get("Accept-Language")
	}
	if i := strings.IndexAny(raw, ",;"); i > 0 {
		raw = raw[:i]
	}
	code := langdetect.Normalize(raw)
	if len(code) != 2 {
		return "ru"
	}
	return code
}

func (a *API) handleListNews(w http.ResponseWriter, r *http.Request) {
	f := store.ArticleFilter{
		Lang:     langParam(r),
		Category: strings.TrimSpace(r.URL.Query().Get("category")),
		Source:   strings.TrimSpace(r.URL.Query().Get("source")),
		Tag:      strings.TrimSpace(r.URL.Query().Get("tag")),
		Query:    strings.TrimSpace(r.URL.Query().Get("q")),
		Sort:     strings.TrimSpace(r.URL.Query().Get("sort")),
		Page:     queryInt(r, "page", 1),
		Limit:    queryInt(r, "limit", 20),
		UserID:   currentUserID(r.Context()),
	}
	if f.Category != "" && !news.CategoryExists(f.Category) {
		writeError(w, http.StatusBadRequest, "invalid_category", "unknown category")
		return
	}
	switch f.Sort {
	case "", "latest", "hot", "trending", "oldest":
	default:
		writeError(w, http.StatusBadRequest, "invalid_sort", "sort must be latest, hot, trending or oldest")
		return
	}
	if since, ok := parseSince(r.URL.Query().Get("since")); ok {
		f.Since = since
	}
	a.respondNewsList(w, r, f)
}

func (a *API) handleTrending(w http.ResponseWriter, r *http.Request) {
	f := store.ArticleFilter{
		Lang:   langParam(r),
		Sort:   "hot",
		Page:   1,
		Limit:  queryInt(r, "limit", 10),
		UserID: currentUserID(r.Context()),
		Since:  time.Now().UTC().Add(-7 * 24 * time.Hour),
	}
	if since, ok := parseSince(r.URL.Query().Get("since")); ok {
		f.Since = since
	}
	a.respondNewsList(w, r, f)
}

func (a *API) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		writeError(w, http.StatusBadRequest, "validation_error", "query must contain at least 2 characters")
		return
	}
	f := store.ArticleFilter{
		Lang:   langParam(r),
		Query:  q,
		Sort:   strings.TrimSpace(r.URL.Query().Get("sort")),
		Page:   queryInt(r, "page", 1),
		Limit:  queryInt(r, "limit", 20),
		UserID: currentUserID(r.Context()),
	}
	a.respondNewsList(w, r, f)
}

func (a *API) respondNewsList(w http.ResponseWriter, r *http.Request, f store.ArticleFilter) {
	f.Page, f.Limit = normalizePage(f.Page, f.Limit)
	items, total, err := a.st.ListArticles(r.Context(), f)
	if err != nil {
		a.log.Error("list articles", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load news")
		return
	}
	writeList(w, http.StatusOK, toArticleDTOs(items, f.Lang, false), newMeta(f.Page, f.Limit, total))
}

func (a *API) handleGetNews(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	lang := langParam(r)
	userID := currentUserID(r.Context())
	article, err := a.st.GetArticle(r.Context(), id, lang, userID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := a.st.IncrementViews(r.Context(), id, userID); err == nil {
		article.ViewsCount++
	}
	dto := toArticleDTO(*article, lang, true)
	if related, relErr := a.st.RelatedArticles(r.Context(), id, article.Category, lang, userID, 5); relErr == nil {
		dto.Related = toArticleDTOs(related, lang, false)
	}
	writeData(w, http.StatusOK, dto)
}

func (a *API) handleCategories(w http.ResponseWriter, r *http.Request) {
	lang := langParam(r)
	type categoryDTO struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	out := make([]categoryDTO, 0, len(news.Categories))
	for _, c := range news.Categories {
		name := c.NameRU
		if lang == "en" {
			name = c.NameEN
		}
		out = append(out, categoryDTO{Slug: c.Slug, Name: name})
	}
	writeData(w, http.StatusOK, out)
}

func (a *API) handleSources(w http.ResponseWriter, r *http.Request) {
	sources, err := a.st.ListSources(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	out := make([]*sourceDTO, 0, len(sources))
	for i := range sources {
		out = append(out, toSourceDTO(&sources[i]))
	}
	writeData(w, http.StatusOK, out)
}

func (a *API) handleTags(w http.ResponseWriter, r *http.Request) {
	lang := langParam(r)
	tags, err := a.st.ListTags(r.Context(), queryInt(r, "limit", 100))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	type tagCountDTO struct {
		tagDTO
		Count int `json:"count"`
	}
	out := make([]tagCountDTO, 0, len(tags))
	for _, t := range tags {
		out = append(out, tagCountDTO{tagDTO: toTagDTO(t.Tag, lang), Count: t.Count})
	}
	writeData(w, http.StatusOK, out)
}

func (a *API) handleTag(w http.ResponseWriter, r *http.Request) {
	lang := langParam(r)
	tag, err := a.st.GetTagBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, toTagDTO(*tag, lang))
}

func parseSince(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	if d, err := time.ParseDuration(v); err == nil {
		return time.Now().UTC().Add(-d), true
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}
