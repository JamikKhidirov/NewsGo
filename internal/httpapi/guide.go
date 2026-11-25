package httpapi

import (
	"bytes"
	"embed"
	"fmt"
	stdhtml "html"
	"html/template"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/go-chi/chi/v5"
	gonews "github.com/gonews/gonews"
	"github.com/gonews/gonews/docs"
	"github.com/gonews/gonews/internal/version"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed assets/*
var docsAssets embed.FS

// guidePageDef maps a public URL to a Markdown file.
type guidePageDef struct {
	Slug  string
	Title string
	File  string
	Root  bool // file lives in the repository root (README, .env.example)
}

var guidePages = []guidePageDef{
	{Slug: "overview", Title: "Обзор и быстрый старт", File: "ru/OVERVIEW.md"},
	{Slug: "architecture", Title: "Архитектура", File: "ru/ARCHITECTURE.md"},
	{Slug: "api", Title: "REST API", File: "ru/API.md"},
	{Slug: "deployment", Title: "Развёртывание", File: "ru/DEPLOYMENT.md"},
	{Slug: "translation", Title: "Перевод", File: "ru/TRANSLATION.md"},
	{Slug: "aggregators", Title: "Источники и агрегация", File: "ru/AGGREGATORS.md"},
	{Slug: "ci-cd", Title: "CI/CD и релизы", File: "ru/CI-CD.md"},
	{Slug: "infrastructure", Title: "PostgreSQL, Redis и Kafka", File: "ru/INFRASTRUCTURE.md"},
	{Slug: "observability", Title: "Мониторинг, логи и алерты", File: "ru/OBSERVABILITY.md"},
	{Slug: "notifications", Title: "Push: Android и iOS", File: "ru/NOTIFICATIONS.md"},
	{Slug: "performance", Title: "Производительность и масштабирование", File: "ru/PERFORMANCE.md"},
	{Slug: "load-testing", Title: "Нагрузочное тестирование", File: "ru/LOAD-TESTING.md"},
}

type tocEntry struct {
	Level int
	ID    string
	Text  string
}

type guidePage struct {
	Slug    string
	Title   string
	Content template.HTML
	TOC     []tocEntry
}

var (
	guideOnce   sync.Once
	guideBySlug map[string]*guidePage
	guideOrder  []*guidePage
	guideErr    error
)

var (
	reTOCHeading = regexp.MustCompile(`(?s)<h([23]) id="([^"]+)">(.*?)</h[23]>`)
	reTags       = regexp.MustCompile(`(?s)<[^>]*>`)
)

// docLinkReplacer rewrites repository-relative links to their web equivalents.
var docLinkReplacer = strings.NewReplacer(
	`href="docs/ru/API.md"`, `href="/docs/api"`,
	`href="docs/ru/ARCHITECTURE.md"`, `href="/docs/architecture"`,
	`href="docs/ru/DEPLOYMENT.md"`, `href="/docs/deployment"`,
	`href="docs/ru/TRANSLATION.md"`, `href="/docs/translation"`,
	`href="docs/ru/AGGREGATORS.md"`, `href="/docs/aggregators"`,
	`href="docs/ru/CI-CD.md"`, `href="/docs/ci-cd"`,
	`href="docs/ru/INFRASTRUCTURE.md"`, `href="/docs/infrastructure"`,
	`href="docs/ru/OBSERVABILITY.md"`, `href="/docs/observability"`,
	`href="docs/ru/NOTIFICATIONS.md"`, `href="/docs/notifications"`,
	`href="docs/ru/PERFORMANCE.md"`, `href="/docs/performance"`,
	`href="docs/ru/LOAD-TESTING.md"`, `href="/docs/load-testing"`,
	`href="docs/api/postman_collection.json"`, `href="/docs/postman_collection.json"`,
	`href="docs/api/postman_environment.json"`, `href="/docs/postman_environment.json"`,
	`href="docs/API.md"`, `href="/docs/api"`,
	`href="docs/ARCHITECTURE.md"`, `href="/docs/architecture"`,
	`href="docs/DEPLOYMENT.md"`, `href="/docs/deployment"`,
	`href="docs/TRANSLATION.md"`, `href="/docs/translation"`,
	`href="docs/AGGREGATORS.md"`, `href="/docs/aggregators"`,
	`href="docs/CI-CD.md"`, `href="/docs/ci-cd"`,
	`href="docs/postman_collection.json"`, `href="/docs/postman_collection.json"`,
	`href="docs/postman_environment.json"`, `href="/docs/postman_environment.json"`,
	`href="API.md"`, `href="/docs/api"`,
	`href="ARCHITECTURE.md"`, `href="/docs/architecture"`,
	`href="DEPLOYMENT.md"`, `href="/docs/deployment"`,
	`href="TRANSLATION.md"`, `href="/docs/translation"`,
	`href="AGGREGATORS.md"`, `href="/docs/aggregators"`,
	`href="CI-CD.md"`, `href="/docs/ci-cd"`,
	`href="INFRASTRUCTURE.md"`, `href="/docs/infrastructure"`,
	`href="OBSERVABILITY.md"`, `href="/docs/observability"`,
	`href="NOTIFICATIONS.md"`, `href="/docs/notifications"`,
	`href="PERFORMANCE.md"`, `href="/docs/performance"`,
	`href="LOAD-TESTING.md"`, `href="/docs/load-testing"`,
	`href="ru/API.md"`, `href="/docs/api"`,
	`href="ru/ARCHITECTURE.md"`, `href="/docs/architecture"`,
	`href="ru/DEPLOYMENT.md"`, `href="/docs/deployment"`,
	`href="ru/TRANSLATION.md"`, `href="/docs/translation"`,
	`href="ru/AGGREGATORS.md"`, `href="/docs/aggregators"`,
	`href="ru/CI-CD.md"`, `href="/docs/ci-cd"`,
	`href="ru/INFRASTRUCTURE.md"`, `href="/docs/infrastructure"`,
	`href="ru/OBSERVABILITY.md"`, `href="/docs/observability"`,
	`href="ru/NOTIFICATIONS.md"`, `href="/docs/notifications"`,
	`href="ru/PERFORMANCE.md"`, `href="/docs/performance"`,
	`href="ru/LOAD-TESTING.md"`, `href="/docs/load-testing"`,
	`href="README.md"`, `href="/docs"`,
	`href=".env.example"`, `href="/docs/env.example"`,
	`href="LICENSE"`, `href="/docs/LICENSE"`,
	`href="http://localhost:8080/docs"`, `href="/docs"`,
)

// buildGuide renders every Markdown document once, at first request.
func buildGuide() {
	guideOnce.Do(func() {
		md := goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithParserOptions(parser.WithAutoHeadingID()),
			goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
		)
		guideBySlug = make(map[string]*guidePage, len(guidePages))
		for _, def := range guidePages {
			src, err := readGuideSource(def)
			if err != nil {
				guideErr = fmt.Errorf("read %s: %w", def.File, err)
				return
			}
			var buf bytes.Buffer
			ctx := parser.NewContext(parser.WithIDs(newGitHubIDs()))
			if err := md.Convert(src, &buf, parser.WithContext(ctx)); err != nil {
				guideErr = fmt.Errorf("render %s: %w", def.File, err)
				return
			}
			content := docLinkReplacer.Replace(buf.String())
			page := &guidePage{
				Slug:    def.Slug,
				Title:   def.Title,
				Content: template.HTML(content), // trusted, embedded Markdown
				TOC:     extractTOC(content),
			}
			guideBySlug[def.Slug] = page
			guideOrder = append(guideOrder, page)
		}
	})
}

func readGuideSource(def guidePageDef) ([]byte, error) {
	if def.Root {
		return gonews.Content.ReadFile(def.File)
	}
	return fs.ReadFile(docs.FS, def.File)
}

// githubIDs reproduces GitHub-style heading anchors: Cyrillic letters are
// preserved, punctuation is removed and duplicates get a numeric suffix.
// Goldmark's built-in generator drops every non-ASCII rune, which turns
// Russian headings into "heading", "heading-1", ... and breaks the README TOC.
type githubIDs struct {
	used map[string]bool
	next map[string]int
}

func newGitHubIDs() *githubIDs {
	return &githubIDs{used: map[string]bool{}, next: map[string]int{}}
}

func (g *githubIDs) Generate(value []byte, kind ast.NodeKind) []byte {
	base := slugifyHeading(string(value))
	if base == "" {
		if kind == ast.KindHeading {
			base = "heading"
		} else {
			base = "id"
		}
	}
	if !g.used[base] {
		g.used[base] = true
		return []byte(base)
	}
	for i := g.next[base] + 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !g.used[candidate] {
			g.used[candidate] = true
			g.next[base] = i
			return []byte(candidate)
		}
	}
}

func (g *githubIDs) Put(value []byte) {
	g.used[string(value)] = true
}

func slugifyHeading(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || unicode.IsSpace(r):
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func extractTOC(content string) []tocEntry {
	matches := reTOCHeading.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]tocEntry, 0, len(matches))
	for _, m := range matches {
		text := stdhtml.UnescapeString(reTags.ReplaceAllString(m[3], ""))
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		level := 2
		if m[1] == "3" {
			level = 3
		}
		out = append(out, tocEntry{Level: level, ID: m[2], Text: text})
	}
	return out
}

type guideNavItem struct {
	Title  string
	Href   string
	Active bool
}

type guideView struct {
	Title   string
	Slug    string
	Content template.HTML
	TOC     []tocEntry
	Nav     []guideNavItem
	Prev    *guideNavItem
	Next    *guideNavItem
	Version string
}

var guideTemplate = template.Must(template.New("guide").Parse(guideHTML))

// handleGuide renders the documentation page selected by the {page} route.
// /docs (without a page) shows the overview.
func (a *API) handleGuide(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "page")
	if slug == "" {
		slug = "overview"
	}
	buildGuide()
	if guideErr != nil {
		a.log.Error("guide render failed", "error", guideErr)
		writeError(w, http.StatusInternalServerError, "docs_unavailable", "documentation is unavailable")
		return
	}
	page, ok := guideBySlug[slug]
	if !ok {
		a.renderGuideNotFound(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if err := guideTemplate.Execute(w, guideViewModel(page)); err != nil {
		a.log.Error("guide template failed", "error", err)
	}
}

func (a *API) renderGuideNotFound(w http.ResponseWriter) {
	page := &guidePage{
		Slug:  "not-found",
		Title: "Раздел не найден",
		Content: template.HTML(`<h1>Раздел не найден</h1>
<p>Такой страницы документации нет. Вернитесь на <a href="/docs">главную страницу документации</a>.</p>`),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_ = guideTemplate.Execute(w, guideViewModel(page))
}

func guideViewModel(page *guidePage) guideView {
	view := guideView{
		Title:   page.Title,
		Slug:    page.Slug,
		Content: page.Content,
		TOC:     page.TOC,
		Version: version.Version,
	}
	for i, p := range guideOrder {
		href := "/docs/" + p.Slug
		if p.Slug == "overview" {
			href = "/docs"
		}
		view.Nav = append(view.Nav, guideNavItem{
			Title:  p.Title,
			Href:   href,
			Active: p.Slug == page.Slug,
		})
		if p.Slug != page.Slug {
			continue
		}
		if i > 0 {
			prev := guideOrder[i-1]
			view.Prev = &guideNavItem{Title: prev.Title, Href: "/docs/" + prev.Slug}
		}
		if i < len(guideOrder)-1 {
			next := guideOrder[i+1]
			view.Next = &guideNavItem{Title: next.Title, Href: "/docs/" + next.Slug}
		}
	}
	return view
}

// handleDocsAsset serves the embedded CSS/JS of the documentation UI.
func (a *API) handleDocsAsset(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "file")
	contentType, ok := docsAssetTypes[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, err := docsAssets.ReadFile("assets/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
}

var docsAssetTypes = map[string]string{
	"common.css":           "text/css; charset=utf-8",
	"guide.css":            "text/css; charset=utf-8",
	"guide.js":             "text/javascript; charset=utf-8",
	"theme.js":             "text/javascript; charset=utf-8",
	"swagger-theme.css":    "text/css; charset=utf-8",
	"swagger-ui.css":       "text/css; charset=utf-8",
	"swagger-ui-bundle.js": "text/javascript; charset=utf-8",
}

// serveEmbedded writes a file from an embedded filesystem with the given headers.
func serveEmbedded(w http.ResponseWriter, r *http.Request, fsys fs.FS, name, contentType, disposition string) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=300")
	if disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	_, _ = w.Write(data)
}

func (a *API) handlePostmanCollection(w http.ResponseWriter, r *http.Request) {
	serveEmbedded(w, r, docs.FS, "api/postman_collection.json", "application/json; charset=utf-8",
		`attachment; filename="gonews-postman-collection.json"`)
}

func (a *API) handlePostmanEnvironment(w http.ResponseWriter, r *http.Request) {
	serveEmbedded(w, r, docs.FS, "api/postman_environment.json", "application/json; charset=utf-8",
		`attachment; filename="gonews-postman-environment.json"`)
}

func (a *API) handleEnvExample(w http.ResponseWriter, r *http.Request) {
	serveEmbedded(w, r, gonews.Content, ".env.example", "text/plain; charset=utf-8", "")
}

func (a *API) handleLicense(w http.ResponseWriter, r *http.Request) {
	serveEmbedded(w, r, gonews.Content, "LICENSE", "text/plain; charset=utf-8", "")
}

const guideHTML = `<!doctype html>
<html lang="ru" data-theme="light">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>{{.Title}} — документация GoNews</title>
<link rel="stylesheet" href="/docs/assets/common.css">
<link rel="stylesheet" href="/docs/assets/guide.css">
<script>try{var t=localStorage.getItem('gonews-theme');document.documentElement.setAttribute('data-theme',t||(window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'));}catch(e){}</script>
</head>
<body>
<header class="docs-header">
  <button type="button" class="menu-btn" id="menu-toggle" aria-label="Открыть меню" aria-controls="sidebar" aria-expanded="false">
    <span></span><span></span><span></span>
  </button>
  <a class="docs-brand" href="/docs">
    <span class="docs-logo" aria-hidden="true">G</span>
    <span class="docs-brand-text">GoNews <span class="docs-badge">docs</span></span>
  </a>
  <nav class="docs-topnav" aria-label="Основные ссылки">
    <a href="/docs/swagger">Swagger UI</a>
    <a href="/docs/openapi.yaml">OpenAPI</a>
    <a href="/docs/postman_collection.json">Postman</a>
    <button type="button" class="theme-toggle" data-theme-toggle aria-label="Переключить тему" title="Переключить тему">
      <span class="theme-icon" aria-hidden="true">🌙</span><span class="theme-label">Тёмная</span>
    </button>
  </nav>
</header>

<div class="docs-layout">
  <aside class="docs-sidebar" id="sidebar" aria-label="Разделы документации">
    <p class="nav-title">Документация</p>
    <nav class="sidebar-nav">
      {{range .Nav}}<a class="nav-link{{if .Active}} active{{end}}" href="{{.Href}}"{{if .Active}} aria-current="page"{{end}}>{{.Title}}</a>
      {{end}}
    </nav>
    <p class="nav-title">Инструменты</p>
    <nav class="sidebar-nav">
      <a class="nav-link" href="/docs/swagger">Swagger UI</a>
      <a class="nav-link" href="/docs/openapi.yaml">OpenAPI 3.0 (YAML)</a>
      <a class="nav-link" href="/docs/postman_collection.json">Postman-коллекция</a>
      <a class="nav-link" href="/docs/postman_environment.json">Postman-окружение</a>
      <a class="nav-link" href="/docs/env.example">.env.example</a>
    </nav>
    <p class="nav-title">Ссылки</p>
    <nav class="sidebar-nav">
      <a class="nav-link" href="/">Главная API</a>
      <a class="nav-link" href="/healthz">Состояние сервиса</a>
      <a class="nav-link" href="/metrics">Метрики</a>
    </nav>
  </aside>
  <div class="sidebar-backdrop" id="sidebar-backdrop" hidden></div>

  <main class="docs-content" id="content">
    <p class="docs-crumbs"><a href="/docs">Документация</a><span aria-hidden="true">/</span>{{.Title}}</p>
    <article class="markdown">
      {{.Content}}
    </article>
    <nav class="docs-pager" aria-label="Навигация по документации">
      {{if .Prev}}<a class="pager-link prev" href="{{.Prev.Href}}"><span>← Назад</span><strong>{{.Prev.Title}}</strong></a>{{else}}<span></span>{{end}}
      {{if .Next}}<a class="pager-link next" href="{{.Next.Href}}"><span>Далее →</span><strong>{{.Next.Title}}</strong></a>{{end}}
    </nav>
    <footer class="docs-footer">
      <span>GoNews {{.Version}} · документация</span>
      <span><a href="/docs/swagger">Swagger UI</a> · <a href="/docs/openapi.yaml">OpenAPI</a></span>
    </footer>
  </main>

  {{if .TOC}}
  <aside class="docs-toc" aria-label="Содержание страницы">
    <p class="toc-title">На этой странице</p>
    <nav>
      {{range .TOC}}<a class="toc-link toc-l{{.Level}}" href="#{{.ID}}">{{.Text}}</a>
      {{end}}
    </nav>
  </aside>
  {{end}}
</div>
<script src="/docs/assets/theme.js"></script>
<script src="/docs/assets/guide.js"></script>
</body>
</html>`
