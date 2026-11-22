package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gonews/gonews/internal/model"
)

// EnsureTags inserts any missing tags (idempotent).
func (s *Store) EnsureTags(ctx context.Context, tags []model.Tag) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tags(id, slug, name_ru, name_en, created_at)
			VALUES(?,?,?,?,?) ON CONFLICT(slug) DO UPDATE SET name_ru=excluded.name_ru, name_en=excluded.name_en`,
			t.ID, t.Slug, t.NameRU, t.NameEN, now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListTags(ctx context.Context, limit int) ([]model.TagCount, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.slug, t.name_ru, t.name_en, COUNT(at.article_id) AS cnt
		FROM tags t LEFT JOIN article_tags at ON at.tag_id=t.id
		GROUP BY t.id, t.slug, t.name_ru, t.name_en
		ORDER BY cnt DESC, t.slug ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.TagCount
	for rows.Next() {
		var tc model.TagCount
		if err := rows.Scan(&tc.ID, &tc.Slug, &tc.NameRU, &tc.NameEN, &tc.Count); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

func (s *Store) GetTagBySlug(ctx context.Context, slug string) (*model.Tag, error) {
	var t model.Tag
	err := s.db.QueryRowContext(ctx, `SELECT id, slug, name_ru, name_en FROM tags WHERE slug=?`, slug).
		Scan(&t.ID, &t.Slug, &t.NameRU, &t.NameEN)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) FollowTag(ctx context.Context, userID, tagID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO follows(user_id, tag_id, created_at) VALUES(?,?,?)`, userID, tagID, now())
	return err
}

func (s *Store) UnfollowTag(ctx context.Context, userID, tagID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM follows WHERE user_id=? AND tag_id=?`, userID, tagID)
	return err
}

func (s *Store) ListFollows(ctx context.Context, userID string) ([]model.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.slug, t.name_ru, t.name_en
		FROM follows f JOIN tags t ON t.id=f.tag_id WHERE f.user_id=? ORDER BY t.slug`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.Slug, &t.NameRU, &t.NameEN); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CountTags returns the number of tags (used in admin stats).
func (s *Store) CountTags(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM tags`).Scan(&n)
	return n, err
}

// --- likes ---

func (s *Store) LikeArticle(ctx context.Context, userID, articleID string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO likes(user_id, article_id, created_at) VALUES(?,?,?)`, userID, articleID, now()); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE articles SET likes_count=(SELECT COUNT(1) FROM likes WHERE article_id=?) WHERE id=?`, articleID, articleID); err != nil {
		return 0, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT likes_count FROM articles WHERE id=?`, articleID).Scan(&count); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) UnlikeArticle(ctx context.Context, userID, articleID string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM likes WHERE user_id=? AND article_id=?`, userID, articleID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE articles SET likes_count=(SELECT COUNT(1) FROM likes WHERE article_id=?) WHERE id=?`, articleID, articleID); err != nil {
		return 0, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT likes_count FROM articles WHERE id=?`, articleID).Scan(&count); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

// --- bookmarks ---

func (s *Store) AddBookmark(ctx context.Context, userID, articleID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO bookmarks(user_id, article_id, created_at) VALUES(?,?,?)`, userID, articleID, now())
	if err != nil {
		return err
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM articles WHERE id=?`, articleID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RemoveBookmark(ctx context.Context, userID, articleID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM bookmarks WHERE user_id=? AND article_id=?`, userID, articleID)
	return err
}

func (s *Store) IsBookmarked(ctx context.Context, userID, articleID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM bookmarks WHERE user_id=? AND article_id=?`, userID, articleID).Scan(&n)
	return n > 0, err
}

// --- comments ---

func (s *Store) ListComments(ctx context.Context, articleID string, page, limit int) ([]model.Comment, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.article_id, c.user_id, c.parent_id, c.body, c.created_at, c.updated_at, c.deleted_at, u.display_name
		FROM comments c JOIN users u ON u.id=c.user_id
		WHERE c.article_id=? ORDER BY c.created_at ASC LIMIT ? OFFSET ?`,
		articleID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.Comment
	for rows.Next() {
		var c model.Comment
		var parent, deleted sql.NullString
		var created, updated string
		if err := rows.Scan(&c.ID, &c.ArticleID, &c.UserID, &parent, &c.Body, &created, &updated, &deleted, &c.AuthorName); err != nil {
			return nil, 0, err
		}
		c.ParentID = nullStr(parent)
		c.CreatedAt = parseTime(created)
		c.UpdatedAt = parseTime(updated)
		if deleted.Valid {
			c.Deleted = true
			c.Body = ""
			c.AuthorName = ""
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM comments WHERE article_id=? AND deleted_at IS NULL`, articleID).Scan(&total); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Store) CreateComment(ctx context.Context, c model.Comment) (*model.Comment, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if c.ParentID != nil {
		var parentArticle string
		err := tx.QueryRowContext(ctx, `SELECT article_id FROM comments WHERE id=? AND deleted_at IS NULL`, *c.ParentID).Scan(&parentArticle)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && parentArticle != c.ArticleID) {
			return nil, ErrNotFound
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO comments(id, article_id, user_id, parent_id, body, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?)`, c.ID, c.ArticleID, c.UserID, c.ParentID, c.Body, now(), now()); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE articles SET comments_count=(SELECT COUNT(1) FROM comments WHERE article_id=? AND deleted_at IS NULL) WHERE id=?`,
		c.ArticleID, c.ArticleID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	c.CreatedAt = nowT()
	c.UpdatedAt = c.CreatedAt
	return &c, nil
}

func (s *Store) UpdateComment(ctx context.Context, id, userID, body string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE comments SET body=?, updated_at=? WHERE id=? AND user_id=? AND deleted_at IS NULL`,
		body, now(), id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteComment(ctx context.Context, id string, userID string, asAdmin bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var articleID, owner string
	err = tx.QueryRowContext(ctx, `SELECT article_id, user_id FROM comments WHERE id=? AND deleted_at IS NULL`, id).Scan(&articleID, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !asAdmin && owner != userID {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE comments SET deleted_at=?, body='' WHERE id=?`, now(), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE articles SET comments_count=(SELECT COUNT(1) FROM comments WHERE article_id=? AND deleted_at IS NULL) WHERE id=?`,
		articleID, articleID); err != nil {
		return err
	}
	return tx.Commit()
}

// --- admin stats ---

func (s *Store) CountArticles(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM articles`).Scan(&n)
	return n, err
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CountComments(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM comments WHERE deleted_at IS NULL`).Scan(&n)
	return n, err
}

// PurgeOlderThan deletes articles older than the given time (retention).
func (s *Store) PurgeOlderThan(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM articles WHERE published_at < ?`, before.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
