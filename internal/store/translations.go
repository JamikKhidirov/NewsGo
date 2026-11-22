package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gonews/gonews/internal/model"
)

// EnqueueTranslation creates a job unless a translation already exists.
func (s *Store) EnqueueTranslation(ctx context.Context, articleID, targetLang string) error {
	var existing int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM article_translations WHERE article_id=? AND lang=?`, articleID, targetLang).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO translation_jobs(article_id, target_lang, status, next_attempt_at, created_at, updated_at)
		VALUES(?,?, 'pending', ?, ?, ?)
		ON CONFLICT(article_id, target_lang) DO UPDATE SET status='pending', next_attempt_at=excluded.next_attempt_at, updated_at=excluded.updated_at`,
		articleID, targetLang, now(), now(), now())
	return err
}

// BackfillTranslations enqueues every article that has no translation for target.
func (s *Store) BackfillTranslations(ctx context.Context, target string, limit int) (int64, error) {
	if limit < 1 || limit > 10000 {
		limit = 1000
	}
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO translation_jobs(article_id, target_lang, status, next_attempt_at, created_at, updated_at)
		SELECT a.id, ?, 'pending', ?, ?, ? FROM articles a
		WHERE a.lang<>? AND NOT EXISTS(SELECT 1 FROM article_translations t WHERE t.article_id=a.id AND t.lang=?)
		LIMIT ?`, target, now(), now(), now(), target, target, limit)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) SaveTranslation(ctx context.Context, articleID, lang, title, summary, content, provider string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO article_translations(article_id, lang, title, summary, content, provider, created_at)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(article_id, lang) DO UPDATE SET title=excluded.title, summary=excluded.summary,
			content=excluded.content, provider=excluded.provider, created_at=excluded.created_at`,
		articleID, lang, title, summary, content, provider, now())
	return err
}

// ClaimTranslationJobs atomically reserves pending jobs for processing.
func (s *Store) ClaimTranslationJobs(ctx context.Context, limit int) ([]model.TranslationJob, error) {
	if limit < 1 {
		limit = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id, article_id, target_lang, attempts FROM translation_jobs
		WHERE status='pending' AND next_attempt_at<=? ORDER BY id ASC LIMIT ?`, now(), limit)
	if err != nil {
		return nil, err
	}
	var jobs []model.TranslationJob
	for rows.Next() {
		var j model.TranslationJob
		if err := rows.Scan(&j.ID, &j.ArticleID, &j.TargetLang, &j.Attempts); err != nil {
			rows.Close()
			return nil, err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, j := range jobs {
		if _, err := tx.ExecContext(ctx, `UPDATE translation_jobs SET status='processing', updated_at=? WHERE id=?`, now(), j.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (s *Store) CompleteTranslationJob(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE translation_jobs SET status='done', last_error='', updated_at=? WHERE id=?`, now(), id)
	return err
}

func (s *Store) FailTranslationJob(ctx context.Context, id int64, attempts int, reason string) error {
	status := "pending"
	next := time.Now().UTC().Add(time.Duration(1<<min(attempts, 6)) * time.Minute)
	if attempts >= 5 {
		status = "failed"
		next = time.Now().UTC().Add(24 * time.Hour)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE translation_jobs SET status=?, attempts=?, last_error=?, next_attempt_at=?, updated_at=? WHERE id=?`,
		status, attempts, truncate(reason, 500), next.Format(time.RFC3339Nano), now(), id)
	return err
}

// ResetTranslationJobs recovers jobs interrupted by a restart.
func (s *Store) ResetTranslationJobs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE translation_jobs SET status='pending', updated_at=? WHERE status='processing'`, now())
	return err
}

func (s *Store) TranslationStats(ctx context.Context) (pending, failed, done int, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(1) FROM translation_jobs GROUP BY status`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err = rows.Scan(&status, &n); err != nil {
			return
		}
		switch status {
		case "pending", "processing":
			pending += n
		case "failed":
			failed += n
		case "done":
			done += n
		}
	}
	err = rows.Err()
	return
}

// GetTranslation returns a stored translation, if any.
func (s *Store) GetTranslation(ctx context.Context, articleID, lang string) (title, summary, content, provider string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT title, summary, content, provider FROM article_translations WHERE article_id=? AND lang=?`,
		articleID, lang).Scan(&title, &summary, &content, &provider)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
