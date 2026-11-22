package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gonews/gonews/internal/model"
)

// UpsertSource registers/updates an aggregator source.
func (s *Store) UpsertSource(ctx context.Context, src model.Source) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sources(id, slug, name, homepage, kind, lang, enabled, created_at)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(slug) DO UPDATE SET name=excluded.name, homepage=excluded.homepage,
			kind=excluded.kind, lang=excluded.lang, enabled=excluded.enabled`,
		src.ID, src.Slug, src.Name, src.Homepage, src.Kind, src.Lang, boolInt(src.Enabled), now())
	return err
}

func (s *Store) ListSources(ctx context.Context) ([]model.Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, slug, name, homepage, kind, lang, enabled FROM sources ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Source
	for rows.Next() {
		var src model.Source
		var enabled int
		if err := rows.Scan(&src.ID, &src.Slug, &src.Name, &src.Homepage, &src.Kind, &src.Lang, &enabled); err != nil {
			return nil, err
		}
		src.Enabled = enabled == 1
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) GetSourceBySlug(ctx context.Context, slug string) (*model.Source, error) {
	var src model.Source
	var enabled int
	err := s.db.QueryRowContext(ctx, `SELECT id, slug, name, homepage, kind, lang, enabled FROM sources WHERE slug=?`, slug).
		Scan(&src.ID, &src.Slug, &src.Name, &src.Homepage, &src.Kind, &src.Lang, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	src.Enabled = enabled == 1
	return &src, nil
}

// UpsertArticle inserts or refreshes an article and its tags. Translation jobs are
// enqueued for every target language different from the article language.
func (s *Store) UpsertArticle(ctx context.Context, a model.Article, tags []model.Tag, targets []string) (string, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `INSERT INTO articles(
			id, source_id, external_id, url, canonical_url, title, summary, content, image_url, author,
			lang, category, published_at, fetched_at, hot_score, unique_hash)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(unique_hash) DO NOTHING`,
		a.ID, a.SourceID, a.ExternalID, a.URL, a.CanonicalURL, a.Title, a.Summary, a.Content, a.ImageURL, a.Author,
		a.Lang, a.Category, a.PublishedAt.UTC().Format(time.RFC3339Nano), a.FetchedAt.UTC().Format(time.RFC3339Nano), a.HotScore, a.UniqueHash)
	if err != nil {
		return "", false, err
	}
	n, _ := res.RowsAffected()
	inserted := n == 1

	articleID := a.ID
	if !inserted {
		if err := tx.QueryRowContext(ctx, `SELECT id FROM articles WHERE unique_hash=?`, a.UniqueHash).Scan(&articleID); err != nil {
			return "", false, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE articles SET
				title=?, summary=?, content=?,
				image_url=CASE WHEN ?<>'' THEN ? ELSE image_url END,
				author=CASE WHEN ?<>'' THEN ? ELSE author END,
				published_at=?, fetched_at=?, hot_score=?, category=?,
				external_id=CASE WHEN ?<>'' THEN ? ELSE external_id END
			WHERE id=?`,
			a.Title, a.Summary, a.Content, a.ImageURL, a.ImageURL, a.Author, a.Author,
			a.PublishedAt.UTC().Format(time.RFC3339Nano), a.FetchedAt.UTC().Format(time.RFC3339Nano), a.HotScore, a.Category,
			a.ExternalID, a.ExternalID, articleID)
		if err != nil {
			return "", false, err
		}
	}

	// Refresh tag links.
	if _, err := tx.ExecContext(ctx, `DELETE FROM article_tags WHERE article_id=?`, articleID); err != nil {
		return "", false, err
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_tags(article_id, tag_id) VALUES(?,?)`, articleID, tag.ID); err != nil {
			return "", false, err
		}
	}

	// Enqueue translation jobs (only for non-native targets).
	for _, target := range targets {
		if target == "" || target == a.Lang {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO translation_jobs(article_id, target_lang, status, next_attempt_at, created_at, updated_at)
			VALUES(?,?, 'pending', ?, ?, ?)
			ON CONFLICT(article_id, target_lang) DO NOTHING`,
			articleID, target, now(), now(), now()); err != nil {
			return "", false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return articleID, inserted, nil
}

// ArticleFilter controls the public news listing.
type ArticleFilter struct {
	Lang       string
	Category   string
	Source     string
	Tag        string
	TagIDs     []string
	Query      string
	Sort       string
	Since      time.Time
	Page       int
	Limit      int
	UserID     string
	Bookmarked bool
	Liked      bool
	History    bool
}

const articleSelect = `
SELECT a.id, a.source_id, a.external_id, a.url, a.canonical_url,
       a.title, a.summary, a.content, a.image_url, a.author, a.lang, a.category,
       a.published_at, a.fetched_at, a.hot_score, a.likes_count, a.comments_count, a.views_count, a.unique_hash,
       CASE WHEN a.lang = ? THEN a.title WHEN tr.article_id IS NOT NULL THEN tr.title ELSE a.title END,
       CASE WHEN a.lang = ? THEN a.summary WHEN tr.article_id IS NOT NULL THEN tr.summary ELSE a.summary END,
       CASE WHEN a.lang = ? THEN a.content WHEN tr.article_id IS NOT NULL THEN tr.content ELSE a.content END,
       CASE WHEN a.lang = ? THEN 'original'
            WHEN tr.article_id IS NOT NULL THEN 'ready'
            WHEN EXISTS(SELECT 1 FROM translation_jobs tj WHERE tj.article_id=a.id AND tj.target_lang=? AND tj.status='failed') THEN 'failed'
            ELSE 'pending' END,
       COALESCE(tr.provider, ''),
       EXISTS(SELECT 1 FROM likes l WHERE l.article_id = a.id AND l.user_id = ?),
       EXISTS(SELECT 1 FROM bookmarks b WHERE b.article_id = a.id AND b.user_id = ?)
FROM articles a
LEFT JOIN article_translations tr ON tr.article_id = a.id AND tr.lang = ?
`

// ListArticles returns localized articles plus the total number of matches.
func (s *Store) ListArticles(ctx context.Context, f ArticleFilter) ([]model.LocalizedArticle, int, error) {
	lang := f.Lang
	if lang == "" {
		lang = "ru"
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}

	var joins []string
	var where []string
	var args []any

	if f.Bookmarked && f.UserID != "" {
		joins = append(joins, `JOIN bookmarks bk ON bk.article_id=a.id AND bk.user_id=?`)
		args = append(args, f.UserID)
	}
	if f.Liked && f.UserID != "" {
		joins = append(joins, `JOIN likes lk ON lk.article_id=a.id AND lk.user_id=?`)
		args = append(args, f.UserID)
	}
	if f.History && f.UserID != "" {
		joins = append(joins, `JOIN article_views av ON av.article_id=a.id AND av.user_id=?`)
		args = append(args, f.UserID)
	}

	if f.Category != "" {
		where = append(where, `a.category=?`)
		args = append(args, f.Category)
	}
	if f.Source != "" {
		where = append(where, `EXISTS(SELECT 1 FROM sources s WHERE s.id=a.source_id AND s.slug=?)`)
		args = append(args, f.Source)
	}
	if f.Tag != "" {
		where = append(where, `EXISTS(SELECT 1 FROM article_tags at JOIN tags t ON t.id=at.tag_id WHERE at.article_id=a.id AND t.slug=?)`)
		args = append(args, f.Tag)
	}
	if len(f.TagIDs) > 0 {
		ph := placeholders(len(f.TagIDs))
		where = append(where, `EXISTS(SELECT 1 FROM article_tags atf WHERE atf.article_id=a.id AND atf.tag_id IN (`+ph+`))`)
		for _, id := range f.TagIDs {
			args = append(args, id)
		}
	}
	if !f.Since.IsZero() {
		where = append(where, `a.published_at >= ?`)
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		where = append(where, `(lower(a.title) LIKE ? OR lower(a.summary) LIKE ? OR lower(a.content) LIKE ? OR lower(tr.title) LIKE ? OR lower(tr.summary) LIKE ?)`)
		args = append(args, like, like, like, like, like)
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	order := ` ORDER BY a.published_at DESC`
	switch f.Sort {
	case "hot", "trending":
		order = ` ORDER BY (a.hot_score + a.likes_count*2.0 + a.comments_count*3.0) DESC, a.published_at DESC`
	case "oldest":
		order = ` ORDER BY a.published_at ASC`
	}
	if f.History {
		order = ` ORDER BY av.viewed_at DESC`
	}
	if f.Bookmarked {
		order = ` ORDER BY bk.created_at DESC`
	}

	query := articleSelect + strings.Join(joins, "\n") + whereSQL + order + ` LIMIT ? OFFSET ?`
	queryArgs := make([]any, 0, 8+len(args)+2)
	queryArgs = append(queryArgs, lang, lang, lang, lang, lang, f.UserID, f.UserID, lang)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, f.Limit, (f.Page-1)*f.Limit)

	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list articles: %w", err)
	}
	defer rows.Close()

	items, err := scanLocalized(rows)
	if err != nil {
		return nil, 0, err
	}
	if err := s.attachTagsAndSources(ctx, items); err != nil {
		return nil, 0, err
	}

	countQuery := `SELECT COUNT(1) FROM articles a LEFT JOIN article_translations tr ON tr.article_id=a.id AND tr.lang=?` +
		strings.Join(joins, "\n") + whereSQL
	countArgs := make([]any, 0, 1+len(args))
	countArgs = append(countArgs, lang)
	countArgs = append(countArgs, args...)
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count articles: %w", err)
	}
	return items, total, nil
}

func scanLocalized(rows *sql.Rows) ([]model.LocalizedArticle, error) {
	var out []model.LocalizedArticle
	for rows.Next() {
		var a model.LocalizedArticle
		var published, fetched string
		var liked, bookmarked int
		err := rows.Scan(
			&a.ID, &a.SourceID, &a.ExternalID, &a.URL, &a.CanonicalURL,
			&a.Title, &a.Summary, &a.Content, &a.ImageURL, &a.Author, &a.Lang, &a.Category,
			&published, &fetched, &a.HotScore, &a.LikesCount, &a.CommentsCount, &a.ViewsCount, &a.UniqueHash,
			&a.DisplayTitle, &a.DisplaySummary, &a.DisplayContent, &a.TranslationStatus, &a.TranslationProvider,
			&liked, &bookmarked,
		)
		if err != nil {
			return nil, err
		}
		a.PublishedAt = parseTime(published)
		a.FetchedAt = parseTime(fetched)
		a.Liked = liked == 1
		a.Bookmarked = bookmarked == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) attachTagsAndSources(ctx context.Context, items []model.LocalizedArticle) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]any, 0, len(items))
	index := make(map[string]int, len(items))
	for i := range items {
		ids = append(ids, items[i].ID)
		index[items[i].ID] = i
	}

	sources, err := s.ListSources(ctx)
	if err != nil {
		return err
	}
	srcMap := make(map[string]*model.Source, len(sources))
	for i := range sources {
		src := sources[i]
		srcMap[src.ID] = &src
	}
	for i := range items {
		items[i].Source = srcMap[items[i].SourceID]
	}

	rows, err := s.db.QueryContext(ctx, `SELECT at.article_id, t.id, t.slug, t.name_ru, t.name_en
		FROM article_tags at JOIN tags t ON t.id=at.tag_id
		WHERE at.article_id IN (`+placeholders(len(ids))+`)
		ORDER BY t.slug`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var articleID string
		var tag model.Tag
		if err := rows.Scan(&articleID, &tag.ID, &tag.Slug, &tag.NameRU, &tag.NameEN); err != nil {
			return err
		}
		if i, ok := index[articleID]; ok {
			items[i].Tags = append(items[i].Tags, tag)
		}
	}
	return rows.Err()
}

// GetArticle returns one localized article.
func (s *Store) GetArticle(ctx context.Context, id, lang, userID string) (*model.LocalizedArticle, error) {
	if lang == "" {
		lang = "ru"
	}
	query := articleSelect + ` WHERE a.id=? LIMIT 1`
	rows, err := s.db.QueryContext(ctx, query, lang, lang, lang, lang, lang, userID, userID, lang, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanLocalized(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	if err := s.attachTagsAndSources(ctx, items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (s *Store) GetArticleRaw(ctx context.Context, id string) (*model.Article, error) {
	var a model.Article
	var published, fetched string
	err := s.db.QueryRowContext(ctx, `SELECT id, source_id, external_id, url, canonical_url, title, summary, content,
		image_url, author, lang, category, published_at, fetched_at, hot_score, likes_count, comments_count, views_count, unique_hash
		FROM articles WHERE id=?`, id).Scan(
		&a.ID, &a.SourceID, &a.ExternalID, &a.URL, &a.CanonicalURL, &a.Title, &a.Summary, &a.Content,
		&a.ImageURL, &a.Author, &a.Lang, &a.Category, &published, &fetched, &a.HotScore,
		&a.LikesCount, &a.CommentsCount, &a.ViewsCount, &a.UniqueHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.PublishedAt = parseTime(published)
	a.FetchedAt = parseTime(fetched)
	return &a, nil
}

// RelatedArticles returns recent articles from the same category.
func (s *Store) RelatedArticles(ctx context.Context, id, category, lang, userID string, limit int) ([]model.LocalizedArticle, error) {
	if lang == "" {
		lang = "ru"
	}
	if limit < 1 || limit > 20 {
		limit = 5
	}
	query := articleSelect + ` WHERE a.category=? AND a.id<>? ORDER BY (a.hot_score + a.likes_count*2.0) DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, lang, lang, lang, lang, lang, userID, userID, lang, category, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanLocalized(rows)
	if err != nil {
		return nil, err
	}
	if err := s.attachTagsAndSources(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// IncrementViews counts a view and stores reading history for logged in users.
func (s *Store) IncrementViews(ctx context.Context, articleID, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE articles SET views_count=views_count+1 WHERE id=?`, articleID); err != nil {
		return err
	}
	if userID != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO article_views(user_id, article_id, viewed_at) VALUES(?,?,?)
			ON CONFLICT(user_id, article_id) DO UPDATE SET viewed_at=excluded.viewed_at`, userID, articleID, now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FeedTagIDs derives interests from follows, likes and bookmarks.
func (s *Store) FeedTagIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT tag_id FROM follows WHERE user_id=?
		UNION
		SELECT at.tag_id FROM article_tags at JOIN likes l ON l.article_id=at.article_id WHERE l.user_id=?
		UNION
		SELECT at.tag_id FROM article_tags at JOIN bookmarks b ON b.article_id=at.article_id WHERE b.user_id=?
		LIMIT 20`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
