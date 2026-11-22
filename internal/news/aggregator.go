package news

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gonews/gonews/internal/langdetect"
	"github.com/gonews/gonews/internal/model"
	"github.com/gonews/gonews/internal/store"
)

// ErrAlreadyRunning is returned when an ingestion is already in progress.
var ErrAlreadyRunning = errors.New("ingestion already running")

// ProviderResult reports one provider's contribution.
type ProviderResult struct {
	Fetched  int    `json:"fetched"`
	Inserted int    `json:"inserted"`
	Updated  int    `json:"updated"`
	Error    string `json:"error,omitempty"`
}

// Stats summarizes one ingestion run.
type Stats struct {
	RunID      int64                     `json:"runId"`
	StartedAt  time.Time                 `json:"startedAt"`
	FinishedAt time.Time                 `json:"finishedAt"`
	DurationMS int64                     `json:"durationMs"`
	Fetched    int                       `json:"fetched"`
	Inserted   int                       `json:"inserted"`
	Updated    int                       `json:"updated"`
	Errors     int                       `json:"errors"`
	Providers  map[string]ProviderResult `json:"providers"`
}

// Aggregator fetches, normalizes, deduplicates and stores news.
type Aggregator struct {
	st        *store.Store
	providers []Provider
	targets   []string
	limit     int
	log       *slog.Logger
	running   atomic.Bool
}

// NewAggregator wires the aggregator.
func NewAggregator(st *store.Store, providers []Provider, targets []string, limit int, log *slog.Logger) *Aggregator {
	if log == nil {
		log = slog.Default()
	}
	if limit < 1 {
		limit = 25
	}
	return &Aggregator{st: st, providers: providers, targets: targets, limit: limit, log: log}
}

// Seed registers sources and the tag dictionary.
func (a *Aggregator) Seed(ctx context.Context) error {
	if err := a.st.EnsureTags(ctx, Tags()); err != nil {
		return err
	}
	for _, p := range a.providers {
		if err := a.st.UpsertSource(ctx, p.Source()); err != nil {
			return err
		}
	}
	return nil
}

type fetchResult struct {
	provider Provider
	items    []model.IngestItem
	err      error
}

type normalized struct {
	article      model.Article
	tags         []model.Tag
	providerSlug string
}

// Run performs one full aggregation cycle. Concurrent calls are rejected.
func (a *Aggregator) Run(ctx context.Context) (*Stats, error) {
	if !a.running.CompareAndSwap(false, true) {
		return nil, ErrAlreadyRunning
	}
	defer a.running.Store(false)

	started := time.Now().UTC()
	runID, err := a.st.StartIngestRun(ctx)
	if err != nil {
		return nil, err
	}
	stats := &Stats{RunID: runID, StartedAt: started, Providers: map[string]ProviderResult{}}

	// Phase 1: fetch all providers concurrently, with a global concurrency cap.
	ch := make(chan fetchResult, len(a.providers))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, provider := range a.providers {
		provider := provider
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fetchCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
			defer cancel()
			items, fetchErr := provider.Fetch(fetchCtx, a.limit)
			ch <- fetchResult{provider: provider, items: items, err: fetchErr}
		}()
	}
	wg.Wait()
	close(ch)

	results := make([]fetchResult, 0, len(a.providers))
	for r := range ch {
		results = append(results, r)
	}

	// Phase 2: normalize + cross-source dedupe by canonical URL hash.
	defBySlug := make(map[string]SourceDef, len(SourceDefs))
	for _, def := range SourceDefs {
		defBySlug[def.Slug] = def
	}
	now := time.Now().UTC()
	chosen := map[string]normalized{}
	order := make([]string, 0, 400)
	successfulProviders := 0

	for _, r := range results {
		slug := r.provider.Source().Slug
		pr := ProviderResult{Fetched: len(r.items)}
		if r.err != nil {
			pr.Error = r.err.Error()
			stats.Errors++
			stats.Providers[slug] = pr
			a.log.Warn("provider fetch failed", "source", slug, "error", r.err)
			continue
		}
		successfulProviders++
		stats.Fetched += len(r.items)
		stats.Providers[slug] = pr

		for _, item := range r.items {
			def, ok := defBySlug[item.SourceSlug]
			if !ok {
				stats.Errors++
				continue
			}
			article, tags, hash, normErr := normalizeItem(item, def, a.targets, now)
			if normErr != nil {
				stats.Errors++
				continue
			}
			if prev, exists := chosen[hash]; exists {
				// Prefer the copy with richer content.
				if len(article.Content) > len(prev.article.Content) {
					chosen[hash] = normalized{article: article, tags: tags, providerSlug: slug}
				}
				continue
			}
			chosen[hash] = normalized{article: article, tags: tags, providerSlug: slug}
			order = append(order, hash)
		}
	}

	// Make sure every extracted tag exists before linking.
	seen := map[string]bool{}
	allTags := make([]model.Tag, 0, 64)
	for _, n := range chosen {
		for _, t := range n.tags {
			if !seen[t.ID] {
				seen[t.ID] = true
				allTags = append(allTags, t)
			}
		}
	}
	if len(allTags) > 0 {
		if err := a.st.EnsureTags(ctx, allTags); err != nil {
			a.log.Error("seed extracted tags", "error", err)
		}
	}

	// Phase 3: persist. SQLite serializes writes, so this stays sequential.
	for _, hash := range order {
		n := chosen[hash]
		_, inserted, upErr := a.st.UpsertArticle(ctx, n.article, n.tags, a.targets)
		if upErr != nil {
			stats.Errors++
			a.log.Warn("upsert article failed", "article", n.article.ID, "error", upErr)
			continue
		}
		pr := stats.Providers[n.providerSlug]
		if inserted {
			stats.Inserted++
			pr.Inserted++
		} else {
			stats.Updated++
			pr.Updated++
		}
		stats.Providers[n.providerSlug] = pr
	}

	stats.FinishedAt = time.Now().UTC()
	stats.DurationMS = stats.FinishedAt.Sub(started).Milliseconds()

	status := "ok"
	switch {
	case successfulProviders == 0 && len(a.providers) > 0:
		status = "failed"
	case stats.Errors > 0:
		status = "partial"
	}
	details, _ := json.Marshal(stats.Providers)
	if err := a.st.FinishIngestRun(ctx, runID, status, stats.Fetched, stats.Inserted, stats.Updated, stats.Errors, string(details)); err != nil {
		a.log.Error("finish ingest run", "error", err)
	}
	a.log.Info("ingestion finished",
		"run", runID, "status", status, "fetched", stats.Fetched,
		"inserted", stats.Inserted, "updated", stats.Updated,
		"errors", stats.Errors, "duration_ms", stats.DurationMS)
	return stats, nil
}

func hotScore(published time.Time, def SourceDef, engagement float64, now time.Time) float64 {
	ageHours := now.Sub(published).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	eng := math.Log1p(math.Max(0, engagement)) * 3
	if eng > 25 {
		eng = 25
	}
	score := def.Weight + eng + 60*math.Exp(-ageHours/36)
	return math.Round(score*100) / 100
}

func normalizeItem(item model.IngestItem, def SourceDef, targets []string, now time.Time) (model.Article, []model.Tag, string, error) {
	title := CleanTitle(item.Title)
	if title == "" {
		return model.Article{}, nil, "", errors.New("empty title")
	}
	canonical := CanonicalURL(item.URL)
	if canonical == "" && item.ExternalID == "" {
		return model.Article{}, nil, "", errors.New("empty url")
	}
	hashInput := canonical
	if hashInput == "" {
		hashInput = item.SourceSlug + "|" + item.ExternalID
	}
	hash := URLHash(hashInput)

	lang := langdetect.Normalize(item.Lang)
	if detected := langdetect.Detect(title + " " + item.Summary); detected != "" {
		lang = detected
	}
	if lang == "" {
		lang = def.Lang
	}

	summary := Truncate(StripHTML(item.Summary), 600)
	content := Truncate(StripHTML(item.Content), 20000)
	if summary == "" {
		summary = Truncate(content, 600)
	}
	if content == "" {
		content = summary
	}

	published := item.PublishedAt
	if published.IsZero() {
		published = now
	}
	if published.After(now.Add(2 * time.Hour)) {
		published = now
	}

	category := item.Category
	if !CategoryExists(category) {
		category = DetectCategory(title+" "+summary, def.Category)
	}
	tags := ensureCategoryTag(ExtractTags(title+" "+summary), category)

	a := model.Article{
		ID:           ArticleID(hash),
		SourceID:     SourceID(def.Slug),
		ExternalID:   item.ExternalID,
		URL:          item.URL,
		CanonicalURL: canonical,
		Title:        title,
		Summary:      summary,
		Content:      content,
		ImageURL:     item.ImageURL,
		Author:       strings.TrimSpace(item.Author),
		Lang:         lang,
		Category:     category,
		PublishedAt:  published.UTC(),
		FetchedAt:    now,
		HotScore:     hotScore(published, def, item.Score, now),
		UniqueHash:   hash,
	}
	return a, tags, hash, nil
}

func ensureCategoryTag(tags []model.Tag, category string) []model.Tag {
	for _, t := range tags {
		if t.Slug == category {
			return tags
		}
	}
	if def, ok := TagBySlug(category); ok {
		tags = append(tags, model.Tag{ID: TagID(def.Slug), Slug: def.Slug, NameRU: def.NameRU, NameEN: def.NameEN})
	}
	return tags
}
