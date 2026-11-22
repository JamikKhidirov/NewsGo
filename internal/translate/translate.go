// Package translate implements keyless machine translation with provider
// fallback: Google (unofficial endpoint), Lingva, MyMemory and LibreTranslate.
package translate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gonews/gonews/internal/model"
	"github.com/gonews/gonews/internal/store"
)

// Provider is one translation backend.
type Provider interface {
	Name() string
	MaxChars() int
	Translate(ctx context.Context, text, source, target string) (string, error)
}

// Translator tries providers in order until one succeeds.
type Translator struct {
	providers []Provider
	client    *http.Client
	log       *slog.Logger
	maxChars  int

	mu       sync.Mutex
	cooldown map[string]time.Time
}

// New builds a translator from the configured provider names.
func New(names []string, libreURL, lingvaURL string, timeout time.Duration, maxChars int, log *slog.Logger) *Translator {
	if log == nil {
		log = slog.Default()
	}
	client := &http.Client{Timeout: timeout}
	if timeout <= 0 {
		client.Timeout = 12 * time.Second
	}
	var providers []Provider
	for _, name := range names {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "google":
			providers = append(providers, &googleProvider{client: client})
		case "lingva":
			providers = append(providers, &lingvaProvider{client: client, base: strings.TrimRight(lingvaURL, "/")})
		case "mymemory":
			providers = append(providers, &myMemoryProvider{client: client})
		case "libretranslate":
			providers = append(providers, &libreProvider{client: client, base: strings.TrimRight(libreURL, "/")})
		}
	}
	return &Translator{providers: providers, client: client, log: log, maxChars: maxChars, cooldown: map[string]time.Time{}}
}

// available implements a small circuit breaker: a provider that failed is
// skipped for two minutes, so fallback is fast instead of waiting for the
// same timeout on every field of every article.
func (t *Translator) available(p Provider) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	until, ok := t.cooldown[p.Name()]
	return !ok || time.Now().After(until)
}

func (t *Translator) markFailure(p Provider) {
	t.mu.Lock()
	t.cooldown[p.Name()] = time.Now().Add(2 * time.Minute)
	t.mu.Unlock()
}

func (t *Translator) markSuccess(p Provider) {
	t.mu.Lock()
	delete(t.cooldown, p.Name())
	t.mu.Unlock()
}

// Enabled reports whether at least one provider is configured.
func (t *Translator) Enabled() bool { return len(t.providers) > 0 }

// Translate returns the translated text and the provider that handled it.
func (t *Translator) Translate(ctx context.Context, text, source, target string) (string, string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", nil
	}
	if t.maxChars > 0 {
		if runes := []rune(text); len(runes) > t.maxChars {
			text = strings.TrimSpace(string(runes[:t.maxChars]))
		}
	}
	if source == "" {
		source = "auto"
	}
	if source == target {
		return text, "original", nil
	}
	providers := t.providers
	if len(providers) > 1 {
		var healthy []Provider
		for _, p := range providers {
			if t.available(p) {
				healthy = append(healthy, p)
			}
		}
		if len(healthy) > 0 {
			providers = healthy
		}
	}

	var lastErr error
	for _, p := range providers {
		limit := p.MaxChars()
		if limit <= 0 {
			limit = 4500
		}
		chunks := splitText(text, limit)
		var b strings.Builder
		ok := true
		for i, chunk := range chunks {
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			default:
			}
			out, err := p.Translate(ctx, chunk, source, target)
			if err != nil {
				lastErr = err
				t.markFailure(p)
				t.log.Warn("translation provider failed", "provider", p.Name(), "error", err)
				ok = false
				break
			}
			if i > 0 && !strings.HasSuffix(b.String(), " ") {
				b.WriteString(" ")
			}
			b.WriteString(strings.TrimSpace(out))
		}
		if ok && strings.TrimSpace(b.String()) != "" {
			t.markSuccess(p)
			return b.String(), p.Name(), nil
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no translation provider configured")
	}
	return "", "", lastErr
}

// splitText cuts text into chunks of at most max runes, preferring sentence
// and word boundaries.
func splitText(text string, max int) []string {
	runes := []rune(text)
	if max < 50 {
		max = 50
	}
	if len(runes) <= max {
		return []string{text}
	}
	var chunks []string
	for len(runes) > 0 {
		if len(runes) <= max {
			chunks = append(chunks, string(runes))
			break
		}
		cut := max
		for i := max; i > max/2; i-- {
			if runes[i-1] == '.' || runes[i-1] == '!' || runes[i-1] == '?' || runes[i-1] == '\n' {
				cut = i
				break
			}
		}
		if cut == max {
			for i := max; i > max/2; i-- {
				if runes[i-1] == ' ' || runes[i-1] == ',' {
					cut = i
					break
				}
			}
		}
		chunks = append(chunks, strings.TrimSpace(string(runes[:cut])))
		runes = runes[cut:]
	}
	return chunks
}

func readJSON(resp *http.Response, dst any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, dst)
}

func doJSON(ctx context.Context, client *http.Client, req *http.Request, dst any) error {
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GoNews/1.0; +https://github.com/gonews/gonews)")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	return readJSON(resp, dst)
}

// --- Google Translate (unofficial, keyless) ---

type googleProvider struct{ client *http.Client }

func (p *googleProvider) Name() string  { return "google" }
func (p *googleProvider) MaxChars() int { return 4000 }

func (p *googleProvider) Translate(ctx context.Context, text, source, target string) (string, error) {
	// The clients5 endpoint is a lightweight, keyless Google endpoint that
	// returns ["translated text"]. It is more tolerant of automated clients
	// than translate.googleapis.com.
	q := url.Values{}
	q.Set("client", "dict-chrome-ex")
	q.Set("sl", source)
	q.Set("tl", target)
	q.Set("q", text)
	req, err := http.NewRequest(http.MethodGet, "https://clients5.google.com/translate_a/t?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	var raw any
	if err := doJSON(ctx, p.client, req, &raw); err != nil {
		return "", err
	}
	var b strings.Builder
	collectStrings(raw, &b)
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", errors.New("google: empty translation")
	}
	return out, nil
}

// collectStrings walks Google's nested JSON arrays and joins text fragments.
func collectStrings(v any, b *strings.Builder) {
	switch x := v.(type) {
	case string:
		b.WriteString(x)
	case []any:
		for _, item := range x {
			collectStrings(item, b)
		}
	}
}

// --- Lingva Translate (open source Google proxy) ---

type lingvaProvider struct {
	client *http.Client
	base   string
}

func (p *lingvaProvider) Name() string  { return "lingva" }
func (p *lingvaProvider) MaxChars() int { return 1500 }

func (p *lingvaProvider) Translate(ctx context.Context, text, source, target string) (string, error) {
	if p.base == "" {
		return "", errors.New("lingva: base url is empty")
	}
	endpoint := p.base + "/api/v1/" + url.PathEscape(source) + "/" + url.PathEscape(target) + "/" + url.PathEscape(text)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		Translation string `json:"translation"`
	}
	if err := doJSON(ctx, p.client, req, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.Translation) == "" {
		return "", errors.New("lingva: empty translation")
	}
	return resp.Translation, nil
}

// --- MyMemory (free, keyless, 500 byte limit) ---

type myMemoryProvider struct{ client *http.Client }

func (p *myMemoryProvider) Name() string  { return "mymemory" }
func (p *myMemoryProvider) MaxChars() int { return 450 }

func (p *myMemoryProvider) Translate(ctx context.Context, text, source, target string) (string, error) {
	if source == "auto" {
		source = "en"
	}
	q := url.Values{}
	q.Set("q", text)
	q.Set("langpair", source+"|"+target)
	q.Set("de", "gonews@example.com")
	req, err := http.NewRequest(http.MethodGet, "https://api.mymemory.translated.net/get?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		ResponseData struct {
			TranslatedText string `json:"translatedText"`
		} `json:"responseData"`
		ResponseStatus any `json:"responseStatus"`
	}
	if err := doJSON(ctx, p.client, req, &resp); err != nil {
		return "", err
	}
	out := strings.TrimSpace(resp.ResponseData.TranslatedText)
	if out == "" || strings.Contains(strings.ToUpper(out), "MYMEMORY WARNING") {
		return "", errors.New("mymemory: quota or empty translation")
	}
	return out, nil
}

// --- LibreTranslate (self-hostable) ---

type libreProvider struct {
	client *http.Client
	base   string
}

func (p *libreProvider) Name() string  { return "libretranslate" }
func (p *libreProvider) MaxChars() int { return 3500 }

func (p *libreProvider) Translate(ctx context.Context, text, source, target string) (string, error) {
	if p.base == "" {
		return "", errors.New("libretranslate: base url is empty")
	}
	if source == "auto" {
		source = "auto"
	}
	payload, _ := json.Marshal(map[string]string{
		"q": text, "source": source, "target": target, "format": "text",
	})
	req, err := http.NewRequest(http.MethodPost, p.base+"/translate", strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var resp struct {
		TranslatedText string `json:"translatedText"`
		Error          string `json:"error"`
	}
	if err := doJSON(ctx, p.client, req, &resp); err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", errors.New(resp.Error)
	}
	if strings.TrimSpace(resp.TranslatedText) == "" {
		return "", errors.New("libretranslate: empty translation")
	}
	return resp.TranslatedText, nil
}

// --- Worker: durable translation job queue ---

// Worker drains translation_jobs and stores results in article_translations.
type Worker struct {
	st       *store.Store
	tr       *Translator
	workers  int
	interval time.Duration
	log      *slog.Logger
	notify   chan struct{}
	mu       sync.Mutex
}

// NewWorker builds the translation worker.
func NewWorker(st *store.Store, tr *Translator, workers int, log *slog.Logger) *Worker {
	if workers < 1 {
		workers = 1
	}
	if log == nil {
		log = slog.Default()
	}
	return &Worker{st: st, tr: tr, workers: workers, interval: 15 * time.Second, log: log, notify: make(chan struct{}, 1)}
}

// Notify wakes the worker immediately (called after ingestion).
func (w *Worker) Notify() {
	select {
	case w.notify <- struct{}{}:
	default:
	}
}

// Start runs the worker loop until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	if !w.tr.Enabled() {
		w.log.Info("translation worker disabled (no providers)")
		return
	}
	if err := w.st.ResetTranslationJobs(ctx); err != nil {
		w.log.Warn("reset translation jobs", "error", err)
	}
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-w.notify:
			}
			if n, err := w.RunBatch(ctx, w.workers*2); err != nil {
				w.log.Warn("translation batch", "error", err)
			} else if n > 0 {
				w.log.Info("translated articles", "count", n)
			}
		}
	}()
}

// RunBatch claims and processes up to limit jobs. It returns the number of
// successfully translated articles.
func (w *Worker) RunBatch(ctx context.Context, limit int) (int, error) {
	jobs, err := w.st.ClaimTranslationJobs(ctx, limit)
	if err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, w.workers)
	var success int
	var mu sync.Mutex
	for _, job := range jobs {
		job := job
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if w.processJob(ctx, job) {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return success, nil
}

func (w *Worker) processJob(ctx context.Context, job model.TranslationJob) bool {
	article, err := w.st.GetArticleRaw(ctx, job.ArticleID)
	if err != nil {
		_ = w.st.FailTranslationJob(ctx, job.ID, job.Attempts+1, "article not found")
		return false
	}
	if article.Lang == job.TargetLang {
		_ = w.st.CompleteTranslationJob(ctx, job.ID)
		return true
	}

	title, provider, err := w.tr.Translate(ctx, article.Title, article.Lang, job.TargetLang)
	if err != nil {
		_ = w.st.FailTranslationJob(ctx, job.ID, job.Attempts+1, err.Error())
		return false
	}
	summary, _, err := w.tr.Translate(ctx, article.Summary, article.Lang, job.TargetLang)
	if err != nil {
		_ = w.st.FailTranslationJob(ctx, job.ID, job.Attempts+1, err.Error())
		return false
	}
	content := summary
	if article.Content != "" && article.Content != article.Summary {
		translated, _, cErr := w.tr.Translate(ctx, article.Content, article.Lang, job.TargetLang)
		if cErr != nil {
			_ = w.st.FailTranslationJob(ctx, job.ID, job.Attempts+1, cErr.Error())
			return false
		}
		content = translated
	}
	if err := w.st.SaveTranslation(ctx, article.ID, job.TargetLang, title, summary, content, provider); err != nil {
		_ = w.st.FailTranslationJob(ctx, job.ID, job.Attempts+1, err.Error())
		return false
	}
	if err := w.st.CompleteTranslationJob(ctx, job.ID); err != nil {
		w.log.Warn("complete translation job", "error", err)
	}
	w.log.Debug("article translated", "article", article.ID, "lang", job.TargetLang, "provider", provider)
	return true
}
