package news

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gonews/gonews/internal/langdetect"
	"github.com/gonews/gonews/internal/model"
	"github.com/mmcdole/gofeed"
)

// SourceDef describes a free news source (no API key required).
type SourceDef struct {
	Slug     string
	Name     string
	Homepage string
	Kind     string
	Lang     string
	Category string
	Weight   float64
	URL      string // may contain %d for the item limit
	Parser   string // rss | hackernews | reddit | devto | spaceflight
}

// SourceDefs is the built-in catalog of keyless news sources.
var SourceDefs = []SourceDef{
	{"hackernews", "Hacker News", "https://news.ycombinator.com", "json", "en", "technology", 12, "https://hn.algolia.com/api/v1/search?tags=front_page&hitsPerPage=%d", "hackernews"},
	{"reddit-worldnews", "Reddit r/worldnews", "https://www.reddit.com/r/worldnews", "json", "en", "world", 9, "https://www.reddit.com/r/worldnews/hot.json?limit=%d&raw_json=1", "reddit"},
	{"reddit-technology", "Reddit r/technology", "https://www.reddit.com/r/technology", "json", "en", "technology", 8, "https://www.reddit.com/r/technology/hot.json?limit=%d&raw_json=1", "reddit"},
	{"devto", "DEV Community", "https://dev.to", "json", "en", "technology", 6, "https://dev.to/api/articles?per_page=%d&top=7", "devto"},
	{"spaceflight", "Spaceflight News", "https://spaceflightnewsapi.net", "json", "en", "space", 8, "https://api.spaceflightnewsapi.net/v4/articles/?limit=%d", "spaceflight"},
	{"bbc-world", "BBC News", "https://www.bbc.com/news", "rss", "en", "world", 15, "https://feeds.bbci.co.uk/news/world/rss.xml", "rss"},
	{"npr", "NPR", "https://www.npr.org", "rss", "en", "world", 11, "https://feeds.npr.org/1001/rss.xml", "rss"},
	{"aljazeera", "Al Jazeera", "https://www.aljazeera.com", "rss", "en", "world", 11, "https://www.aljazeera.com/xml/rss/all.xml", "rss"},
	{"guardian-world", "The Guardian", "https://www.theguardian.com", "rss", "en", "world", 13, "https://www.theguardian.com/world/rss", "rss"},
	{"lenta", "Lenta.ru", "https://lenta.ru", "rss", "ru", "world", 13, "https://lenta.ru/rss/news", "rss"},
	{"ria", "РИА Новости", "https://ria.ru", "rss", "ru", "world", 13, "https://ria.ru/export/rss2/archive/index.xml", "rss"},
	{"habr", "Хабр", "https://habr.com", "rss", "ru", "technology", 9, "https://habr.com/ru/rss/news/?fl=ru", "rss"},
	{"kommersant", "Коммерсантъ", "https://www.kommersant.ru", "rss", "ru", "business", 12, "https://www.kommersant.ru/RSS/news.xml", "rss"},
	{"cnews", "CNews", "https://www.cnews.ru", "rss", "ru", "technology", 8, "https://www.cnews.ru/inc/rss/news.xml", "rss"},
	{"coindesk", "CoinDesk", "https://www.coindesk.com", "rss", "en", "crypto", 7, "https://www.coindesk.com/arc/outboundfeeds/rss/", "rss"},
	{"nasa", "NASA", "https://www.nasa.gov", "rss", "en", "space", 10, "https://www.nasa.gov/feed/", "rss"},
	{"techcrunch", "TechCrunch", "https://techcrunch.com", "rss", "en", "technology", 8, "https://techcrunch.com/feed/", "rss"},
	{"theverge", "The Verge", "https://www.theverge.com", "rss", "en", "technology", 8, "https://www.theverge.com/rss/index.xml", "rss"},
	{"arstechnica", "Ars Technica", "https://arstechnica.com", "rss", "en", "technology", 8, "https://feeds.arstechnica.com/arstechnica/index", "rss"},
	{"tass", "ТАСС", "https://tass.ru", "rss", "ru", "world", 12, "https://tass.ru/rss/v2.xml", "rss"},
	{"rbc", "РБК", "https://www.rbc.ru", "rss", "ru", "business", 12, "https://rssexport.rbc.ru/rbcnews/news/30/full.rss", "rss"},
	{"nplus1", "N+1", "https://nplus1.ru", "rss", "ru", "science", 8, "https://nplus1.ru/rss", "rss"},
	{"lobsters", "Lobsters", "https://lobste.rs", "rss", "en", "technology", 6, "https://lobste.rs/rss", "rss"},
	{"bleepingcomputer", "BleepingComputer", "https://www.bleepingcomputer.com", "rss", "en", "security", 7, "https://www.bleepingcomputer.com/feed/", "rss"},
	{"izvestia", "Известия", "https://iz.ru", "rss", "ru", "world", 10, "https://iz.ru/xml/rss/all.xml", "rss"},
	{"arxiv-ai", "arXiv AI", "https://arxiv.org", "rss", "en", "science", 6, "http://export.arxiv.org/api/query?search_query=cat:cs.AI&sortBy=submittedDate&sortOrder=descending&max_results=25", "rss"},
}

// Provider fetches normalized items from one source.
type Provider interface {
	Source() model.Source
	Fetch(ctx context.Context, limit int) ([]model.IngestItem, error)
}

type providerBase struct {
	def    SourceDef
	client *http.Client
	ua     string
}

func (p providerBase) Source() model.Source {
	return model.Source{
		ID:       SourceID(p.def.Slug),
		Slug:     p.def.Slug,
		Name:     p.def.Name,
		Homepage: p.def.Homepage,
		Kind:     p.def.Kind,
		Lang:     p.def.Lang,
		Enabled:  true,
	}
}

func (p providerBase) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", p.ua)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/json, text/xml;q=0.9, */*;q=0.8")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("%s: http %d: %s", p.def.Slug, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// BuildProviders returns enabled providers from the catalog. When only is
// non-empty, only matching slugs are used.
func BuildProviders(only []string, client *http.Client, ua string) []Provider {
	allow := map[string]bool{}
	for _, s := range only {
		allow[strings.TrimSpace(s)] = true
	}
	var out []Provider
	for _, def := range SourceDefs {
		if len(allow) > 0 && !allow[def.Slug] {
			continue
		}
		base := providerBase{def: def, client: client, ua: ua}
		switch def.Parser {
		case "hackernews":
			out = append(out, &hnProvider{providerBase: base})
		case "reddit":
			sub := strings.TrimPrefix(def.Slug, "reddit-")
			out = append(out, &redditProvider{providerBase: base, subreddit: sub})
		case "devto":
			out = append(out, &devtoProvider{providerBase: base})
		case "spaceflight":
			out = append(out, &spaceflightProvider{providerBase: base})
		default:
			out = append(out, &rssProvider{providerBase: base})
		}
	}
	return out
}

// --- RSS / Atom ---

type rssProvider struct{ providerBase }

func (p *rssProvider) Fetch(ctx context.Context, limit int) ([]model.IngestItem, error) {
	body, err := p.fetch(ctx, p.def.URL)
	if err != nil {
		return nil, err
	}
	feed, err := gofeed.NewParser().ParseString(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse rss: %w", err)
	}
	now := time.Now().UTC()
	var items []model.IngestItem
	for i, it := range feed.Items {
		if i >= limit {
			break
		}
		link := strings.TrimSpace(it.Link)
		title := CleanTitle(it.Title)
		if link == "" || title == "" {
			continue
		}
		summary := Truncate(StripHTML(it.Description), 600)
		content := Truncate(StripHTML(it.Content), 20000)
		if content == "" {
			content = Truncate(StripHTML(it.Description), 20000)
		}
		var image string
		if it.Image != nil {
			image = it.Image.URL
		}
		if image == "" {
			for _, enc := range it.Enclosures {
				if enc != nil && strings.HasPrefix(strings.ToLower(enc.Type), "image") {
					image = enc.URL
					break
				}
			}
		}
		image = FirstImage(it.Description+" "+it.Content, image)

		published := now
		if it.PublishedParsed != nil {
			published = it.PublishedParsed.UTC()
		} else if it.UpdatedParsed != nil {
			published = it.UpdatedParsed.UTC()
		}
		if published.After(now.Add(2 * time.Hour)) {
			published = now
		}

		author := ""
		if it.Author != nil {
			author = it.Author.Name
		}
		text := title + " " + summary
		lang := p.def.Lang
		if d := langdetect.Detect(text); d != "" {
			lang = d
		}
		items = append(items, model.IngestItem{
			SourceSlug:  p.def.Slug,
			ExternalID:  firstNonEmpty(it.GUID, link),
			URL:         link,
			Title:       title,
			Summary:     summary,
			Content:     content,
			ImageURL:    image,
			Author:      author,
			Lang:        lang,
			Category:    DetectCategory(text, p.def.Category),
			PublishedAt: published,
		})
	}
	return items, nil
}

// --- Hacker News (Algolia, no key) ---

type hnProvider struct{ providerBase }

type hnResponse struct {
	Hits []struct {
		ObjectID    string    `json:"objectID"`
		Title       string    `json:"title"`
		URL         string    `json:"url"`
		Author      string    `json:"author"`
		CreatedAt   time.Time `json:"created_at"`
		Points      int       `json:"points"`
		StoryText   string    `json:"story_text"`
		CommentText string    `json:"comment_text"`
	} `json:"hits"`
}

func (p *hnProvider) Fetch(ctx context.Context, limit int) ([]model.IngestItem, error) {
	body, err := p.fetch(ctx, fmt.Sprintf(p.def.URL, limit))
	if err != nil {
		return nil, err
	}
	var resp hnResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode hackernews: %w", err)
	}
	var items []model.IngestItem
	for _, h := range resp.Hits {
		title := CleanTitle(h.Title)
		if title == "" {
			continue
		}
		link := h.URL
		if link == "" {
			link = "https://news.ycombinator.com/item?id=" + h.ObjectID
		}
		summary := Truncate(StripHTML(firstNonEmpty(h.StoryText, h.CommentText)), 600)
		items = append(items, model.IngestItem{
			SourceSlug:  p.def.Slug,
			ExternalID:  h.ObjectID,
			URL:         link,
			Title:       title,
			Summary:     summary,
			Content:     Truncate(StripHTML(firstNonEmpty(h.StoryText, h.CommentText)), 20000),
			Author:      h.Author,
			Lang:        "en",
			Category:    DetectCategory(title+" "+summary, p.def.Category),
			PublishedAt: h.CreatedAt.UTC(),
			Score:       float64(h.Points),
		})
	}
	return items, nil
}

// --- Reddit (public JSON, no key) ---

type redditProvider struct {
	providerBase
	subreddit string
}

type redditResponse struct {
	Data struct {
		Children []struct {
			Data struct {
				ID         string  `json:"id"`
				Title      string  `json:"title"`
				URL        string  `json:"url"`
				Permalink  string  `json:"permalink"`
				Selftext   string  `json:"selftext"`
				Thumbnail  string  `json:"thumbnail"`
				Author     string  `json:"author"`
				Subreddit  string  `json:"subreddit"`
				CreatedUTC float64 `json:"created_utc"`
				Score      int     `json:"score"`
				IsSelf     bool    `json:"is_self"`
				PostHint   string  `json:"post_hint"`
				LinkFlair  string  `json:"link_flair_text"`
				Preview    struct {
					Images []struct {
						Source struct {
							URL string `json:"url"`
						} `json:"source"`
					} `json:"images"`
				} `json:"preview"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func (p *redditProvider) Fetch(ctx context.Context, limit int) ([]model.IngestItem, error) {
	body, err := p.fetch(ctx, fmt.Sprintf(p.def.URL, limit))
	if err != nil {
		return nil, err
	}
	var resp redditResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode reddit: %w", err)
	}
	var items []model.IngestItem
	for _, child := range resp.Data.Children {
		d := child.Data
		title := CleanTitle(d.Title)
		if title == "" {
			continue
		}
		link := d.URL
		if d.IsSelf || link == "" || strings.HasPrefix(link, "/") {
			link = "https://www.reddit.com" + d.Permalink
		}
		summary := Truncate(StripHTML(d.Selftext), 600)
		var image string
		if len(d.Preview.Images) > 0 {
			image = d.Preview.Images[0].Source.URL
		}
		if image == "" && strings.HasPrefix(d.Thumbnail, "http") {
			image = d.Thumbnail
		}
		text := title + " " + summary + " " + d.LinkFlair
		items = append(items, model.IngestItem{
			SourceSlug:  p.def.Slug,
			ExternalID:  d.ID,
			URL:         link,
			Title:       title,
			Summary:     summary,
			Content:     Truncate(StripHTML(d.Selftext), 20000),
			ImageURL:    image,
			Author:      d.Author,
			Lang:        "en",
			Category:    DetectCategory(text, p.def.Category),
			PublishedAt: time.Unix(int64(d.CreatedUTC), 0).UTC(),
			Score:       float64(d.Score),
		})
	}
	return items, nil
}

// --- DEV Community (public API, no key) ---

type devtoProvider struct{ providerBase }

type devtoArticle struct {
	ID                     int      `json:"id"`
	Title                  string   `json:"title"`
	Description            string   `json:"description"`
	URL                    string   `json:"url"`
	CoverImage             string   `json:"cover_image"`
	PublishedAt            string   `json:"published_at"`
	TagList                []string `json:"tag_list"`
	PositiveReactionsCount int      `json:"positive_reactions_count"`
	User                   struct {
		Name string `json:"name"`
	} `json:"user"`
}

func (p *devtoProvider) Fetch(ctx context.Context, limit int) ([]model.IngestItem, error) {
	body, err := p.fetch(ctx, fmt.Sprintf(p.def.URL, limit))
	if err != nil {
		return nil, err
	}
	var articles []devtoArticle
	if err := json.Unmarshal(body, &articles); err != nil {
		return nil, fmt.Errorf("decode devto: %w", err)
	}
	var items []model.IngestItem
	for _, a := range articles {
		title := CleanTitle(a.Title)
		if title == "" || a.URL == "" {
			continue
		}
		published, _ := time.Parse(time.RFC3339, a.PublishedAt)
		if published.IsZero() {
			published = time.Now().UTC()
		}
		summary := Truncate(StripHTML(a.Description), 600)
		text := title + " " + summary + " " + strings.Join(a.TagList, " ")
		items = append(items, model.IngestItem{
			SourceSlug:  p.def.Slug,
			ExternalID:  strconv.Itoa(a.ID),
			URL:         a.URL,
			Title:       title,
			Summary:     summary,
			Content:     Truncate(StripHTML(a.Description), 20000),
			ImageURL:    a.CoverImage,
			Author:      a.User.Name,
			Lang:        "en",
			Category:    DetectCategory(text, p.def.Category),
			PublishedAt: published.UTC(),
			Score:       float64(a.PositiveReactionsCount),
		})
	}
	return items, nil
}

// --- Spaceflight News API (no key) ---

type spaceflightProvider struct{ providerBase }

type spaceflightResponse struct {
	Results []struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		URL         string `json:"url"`
		ImageURL    string `json:"image_url"`
		NewsSite    string `json:"news_site"`
		Summary     string `json:"summary"`
		PublishedAt string `json:"published_at"`
	} `json:"results"`
}

func (p *spaceflightProvider) Fetch(ctx context.Context, limit int) ([]model.IngestItem, error) {
	body, err := p.fetch(ctx, fmt.Sprintf(p.def.URL, limit))
	if err != nil {
		return nil, err
	}
	var resp spaceflightResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode spaceflight: %w", err)
	}
	var items []model.IngestItem
	for _, a := range resp.Results {
		title := CleanTitle(a.Title)
		if title == "" || a.URL == "" {
			continue
		}
		published, _ := time.Parse(time.RFC3339, a.PublishedAt)
		if published.IsZero() {
			published = time.Now().UTC()
		}
		summary := Truncate(StripHTML(a.Summary), 600)
		items = append(items, model.IngestItem{
			SourceSlug:  p.def.Slug,
			ExternalID:  strconv.Itoa(a.ID),
			URL:         a.URL,
			Title:       title,
			Summary:     summary,
			Content:     Truncate(StripHTML(a.Summary), 20000),
			ImageURL:    a.ImageURL,
			Author:      a.NewsSite,
			Lang:        "en",
			Category:    "space",
			PublishedAt: published.UTC(),
		})
	}
	return items, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
