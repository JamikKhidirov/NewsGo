package news

import (
	"strings"
	"testing"
	"time"

	"github.com/gonews/gonews/internal/model"
)

func TestStripHTML(t *testing.T) {
	in := `<p>Hello&nbsp;<b>world</b> &amp; friends</p><script>alert(1)</script>`
	got := StripHTML(in)
	if got != "Hello world & friends" {
		t.Fatalf("unexpected strip result: %q", got)
	}
}

func TestCanonicalURL(t *testing.T) {
	got := CanonicalURL("https://Example.COM/news/story/?utm_source=x&utm_medium=y&id=42#top")
	want := "https://example.com/news/story?id=42"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestArticleIDDeterministic(t *testing.T) {
	h1 := URLHash(CanonicalURL("https://example.com/a?utm_source=1"))
	h2 := URLHash(CanonicalURL("https://example.com/a"))
	if h1 != h2 {
		t.Fatal("tracking params must not change the hash")
	}
	if ArticleID(h1) != ArticleID(h2) {
		t.Fatal("article id must be deterministic")
	}
	if !strings.HasPrefix(ArticleID(h1), "a_") {
		t.Fatal("article id must have the a_ prefix")
	}
}

func TestExtractTagsAndCategory(t *testing.T) {
	text := "NASA запустила ракету к Марсу: миссия по изучению климата и космоса"
	tags := ExtractTags(text)
	slugs := map[string]bool{}
	for _, tag := range tags {
		slugs[tag.Slug] = true
	}
	if !slugs["space"] {
		t.Fatalf("expected space tag, got %#v", slugs)
	}
	if !slugs["climate"] {
		t.Fatalf("expected climate tag, got %#v", slugs)
	}
	if cat := DetectCategory(text, "world"); cat != "space" && cat != "climate" {
		t.Fatalf("unexpected category %q", cat)
	}
}

func TestNormalizeItemDedupAndLanguage(t *testing.T) {
	def := SourceDef{Slug: "test", Name: "Test", Lang: "en", Category: "world", Weight: 10}
	item := model.IngestItem{
		SourceSlug:  "test",
		ExternalID:  "1",
		URL:         "https://example.com/story?utm_source=rss",
		Title:       "Scientists discover a new exoplanet",
		Summary:     "The planet orbits a distant star.",
		Lang:        "en",
		PublishedAt: time.Now().Add(-time.Hour),
	}
	a1, tags1, hash1, err := normalizeItem(item, def, []string{"ru"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if a1.Lang != "en" {
		t.Fatalf("expected en, got %q", a1.Lang)
	}
	if len(tags1) == 0 {
		t.Fatal("expected tags")
	}
	item.URL = "https://example.com/story"
	a2, _, hash2, err := normalizeItem(item, def, []string{"ru"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if hash1 != hash2 || a1.ID != a2.ID {
		t.Fatal("URL variants must deduplicate")
	}
	if a1.CanonicalURL != "https://example.com/story" {
		t.Fatalf("unexpected canonical url %q", a1.CanonicalURL)
	}
}

func TestFirstImage(t *testing.T) {
	got := FirstImage(`<img src="https://cdn.example.com/a.jpg">`)
	if got != "https://cdn.example.com/a.jpg" {
		t.Fatalf("unexpected image %q", got)
	}
}
