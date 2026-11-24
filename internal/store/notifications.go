package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gonews/gonews/internal/model"
)

// CreateNotification stores an in-app notification. userID == "" means broadcast.
func (s *Store) CreateNotification(ctx context.Context, n model.Notification) error {
	var userID any
	if n.UserID != nil && *n.UserID != "" {
		userID = *n.UserID
	}
	var articleID any
	if n.ArticleID != nil && *n.ArticleID != "" {
		articleID = *n.ArticleID
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO notifications(id, user_id, kind, title, body, data, article_id, created_at)
		VALUES(?,?,?,?,?,?,?,?)`, n.ID, userID, n.Kind, n.Title, n.Body, n.Data, articleID, now())
	return err
}

func (s *Store) ListNotifications(ctx context.Context, userID string, page, limit int, unreadOnly bool) ([]model.Notification, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 30
	}
	where := ` WHERE (n.user_id IS NULL OR n.user_id=?)`
	args := []any{userID}
	if unreadOnly {
		where += ` AND NOT EXISTS(SELECT 1 FROM notification_reads r WHERE r.notification_id=n.id AND r.user_id=?)`
		args = append(args, userID)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT n.id, n.user_id, n.kind, n.title, n.body, n.data, n.article_id, n.created_at,
			CASE WHEN EXISTS(SELECT 1 FROM notification_reads r WHERE r.notification_id=n.id AND r.user_id=?) THEN 1 ELSE 0 END AS is_read
		FROM notifications n`+where+` ORDER BY n.created_at DESC LIMIT ? OFFSET ?`,
		append([]any{userID}, append(args, limit, (page-1)*limit)...)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.Notification
	for rows.Next() {
		var n model.Notification
		var uid, aid sql.NullString
		var created time.Time
		var read int
		if err := rows.Scan(&n.ID, &uid, &n.Kind, &n.Title, &n.Body, &n.Data, &aid, &created, &read); err != nil {
			return nil, 0, err
		}
		n.UserID = nullStr(uid)
		n.ArticleID = nullStr(aid)
		n.CreatedAt = created.UTC()
		n.Read = read == 1
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	countArgs := []any{}
	countWhere := ` WHERE (n.user_id IS NULL OR n.user_id=?)`
	countArgs = append(countArgs, userID)
	if unreadOnly {
		countWhere += ` AND NOT EXISTS(SELECT 1 FROM notification_reads r WHERE r.notification_id=n.id AND r.user_id=?)`
		countArgs = append(countArgs, userID)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM notifications n`+countWhere, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Store) MarkNotificationRead(ctx context.Context, userID, notificationID string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM notifications WHERE id=? AND (user_id IS NULL OR user_id=?)`, notificationID, userID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO notification_reads(user_id, notification_id, read_at) VALUES(?,?,?) ON CONFLICT DO NOTHING`,
		userID, notificationID, now())
	return err
}

func (s *Store) MarkAllNotificationsRead(ctx context.Context, userID string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO notification_reads(user_id, notification_id, read_at)
		SELECT ?, n.id, ? FROM notifications n
		WHERE (n.user_id IS NULL OR n.user_id=?)
		  AND NOT EXISTS(SELECT 1 FROM notification_reads r WHERE r.notification_id=n.id AND r.user_id=?)
		ON CONFLICT DO NOTHING`,
		userID, now(), userID, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) UnreadNotificationCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM notifications n
		WHERE (n.user_id IS NULL OR n.user_id=?)
		  AND NOT EXISTS(SELECT 1 FROM notification_reads r WHERE r.notification_id=n.id AND r.user_id=?)`,
		userID, userID).Scan(&n)
	return n, err
}

// --- ingest bookkeeping ---

func (s *Store) StartIngestRun(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO ingest_runs(started_at, status) VALUES(?, 'running') RETURNING id`, now()).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) FinishIngestRun(ctx context.Context, id int64, status string, fetched, inserted, updated, errs int, details string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ingest_runs SET finished_at=?, status=?, fetched=?, inserted=?, updated=?, errors=?, details=? WHERE id=?`,
		now(), status, fetched, inserted, updated, errs, details, id)
	return err
}

func (s *Store) ListIngestRuns(ctx context.Context, limit int) ([]model.IngestRun, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, started_at, finished_at, status, fetched, inserted, updated, errors, details
		FROM ingest_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.IngestRun
	for rows.Next() {
		var r model.IngestRun
		var started time.Time
		var finished sql.NullTime
		if err := rows.Scan(&r.ID, &started, &finished, &r.Status, &r.Fetched, &r.Inserted, &r.Updated, &r.Errors, &r.Details); err != nil {
			return nil, err
		}
		r.StartedAt = started.UTC()
		if finished.Valid {
			r.FinishedAt = finished.Time.UTC()
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- settings ---

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// LatestArticleTimes is used by the push broadcaster to avoid duplicate alerts.
func (s *Store) ArticlePublishedAfter(ctx context.Context, after time.Time, limit int) ([]model.LocalizedArticle, error) {
	items, _, err := s.ListArticles(ctx, ArticleFilter{
		Lang:  "ru",
		Since: after,
		Page:  1,
		Limit: limit,
		Sort:  "hot",
	})
	return items, err
}
