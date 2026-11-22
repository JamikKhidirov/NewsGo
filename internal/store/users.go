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

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

// UpsertUser creates a user. Returns ErrConflict when the email already exists.
func (s *Store) UpsertUser(ctx context.Context, u model.User, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(id, email, password_hash, display_name, role, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?)`, u.ID, strings.ToLower(strings.TrimSpace(u.Email)), passwordHash, u.DisplayName, u.Role, now(), now())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrConflict
		}
		return err
	}
	return nil
}

// GetUserByEmail returns the user and its password hash.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*model.User, string, error) {
	var u model.User
	var created, hash string
	err := s.db.QueryRowContext(ctx, `SELECT id, email, password_hash, display_name, role, created_at FROM users WHERE email=?`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&u.ID, &u.Email, &hash, &u.DisplayName, &u.Role, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	u.CreatedAt = parseTime(created)
	return &u, hash, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, email, display_name, role, created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = parseTime(created)
	return &u, nil
}

// UpdatePassword replaces the bcrypt hash for a user.
func (s *Store) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, passwordHash, now(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UserStats contains engagement counters for the profile screen.
type UserStats struct {
	Likes     int `json:"likes"`
	Bookmarks int `json:"bookmarks"`
	Comments  int `json:"comments"`
	Followers int `json:"follows"`
}

func (s *Store) GetUserStats(ctx context.Context, userID string) (UserStats, error) {
	var st UserStats
	err := s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(1) FROM likes WHERE user_id=?),
		(SELECT COUNT(1) FROM bookmarks WHERE user_id=?),
		(SELECT COUNT(1) FROM comments WHERE user_id=? AND deleted_at IS NULL),
		(SELECT COUNT(1) FROM follows WHERE user_id=?)`,
		userID, userID, userID, userID).Scan(&st.Likes, &st.Bookmarks, &st.Comments, &st.Followers)
	return st, err
}

func (s *Store) UpdateUserProfile(ctx context.Context, id, displayName string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET display_name=?, updated_at=? WHERE id=?`, displayName, now(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- refresh tokens ---

func (s *Store) SaveRefreshToken(ctx context.Context, id, userID, tokenHash, userAgent string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO refresh_tokens(id, user_id, token_hash, user_agent, expires_at, created_at)
		VALUES(?,?,?,?,?,?)`, id, userID, tokenHash, userAgent, expiresAt.UTC().Format(time.RFC3339Nano), now())
	return err
}

// RotateRefreshToken atomically revokes the old token and stores a new one.
func (s *Store) RotateRefreshToken(ctx context.Context, oldHash, newID, newHash, userAgent string, expiresAt time.Time) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var userID, exp string
	var revoked sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT user_id, expires_at, revoked_at FROM refresh_tokens WHERE token_hash=?`, oldHash).
		Scan(&userID, &exp, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if revoked.Valid {
		return "", ErrNotFound
	}
	if parseTime(exp).Before(time.Now().UTC()) {
		return "", ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=? WHERE token_hash=?`, now(), oldHash); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO refresh_tokens(id, user_id, token_hash, user_agent, expires_at, created_at)
		VALUES(?,?,?,?,?,?)`, newID, userID, newHash, userAgent, expiresAt.UTC().Format(time.RFC3339Nano), now()); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return userID, nil
}

func (s *Store) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=? WHERE token_hash=? AND revoked_at IS NULL`, now(), tokenHash)
	return err
}

// --- devices ---

func (s *Store) UpsertDevice(ctx context.Context, d model.Device) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO devices(id, user_id, token, platform, locale, topics, enabled, created_at, last_seen_at)
		VALUES(?,?,?,?,?,?,1,?,?)
		ON CONFLICT(token) DO UPDATE SET user_id=excluded.user_id, platform=excluded.platform, locale=excluded.locale,
			topics=excluded.topics, enabled=1, last_seen_at=excluded.last_seen_at`,
		d.ID, d.UserID, d.Token, d.Platform, d.Locale, joinTopics(d.Topics), now(), now())
	return err
}

func (s *Store) DeleteDevice(ctx context.Context, userID, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM devices WHERE user_id=? AND token=?`, userID, token)
	return err
}

// DisableDevice marks a token as invalid after an FCM rejection.
func (s *Store) DisableDevice(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE devices SET enabled=0 WHERE token=?`, token)
	return err
}

func (s *Store) ListDevices(ctx context.Context, userID string) ([]model.Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, token, platform, locale, topics, created_at, last_seen_at
		FROM devices WHERE user_id=? AND enabled=1 ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Device
	for rows.Next() {
		var d model.Device
		var topics, created, seen string
		if err := rows.Scan(&d.ID, &d.UserID, &d.Token, &d.Platform, &d.Locale, &topics, &created, &seen); err != nil {
			return nil, err
		}
		d.Topics = splitTopics(topics)
		d.CreatedAt = parseTime(created)
		d.LastSeenAt = parseTime(seen)
		out = append(out, d)
	}
	return out, rows.Err()
}

// AllDeviceTokens returns all active tokens, optionally filtered by topic.
func (s *Store) AllDeviceTokens(ctx context.Context, topic string) ([]string, error) {
	q := `SELECT token FROM devices WHERE enabled=1`
	args := []any{}
	if topic != "" {
		q += ` AND topics LIKE ?`
		args = append(args, "%\""+topic+"\"%")
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func joinTopics(topics []string) string {
	if len(topics) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(topics))
	for _, t := range topics {
		t = strings.TrimSpace(t)
		if t != "" {
			parts = append(parts, fmt.Sprintf("%q", t))
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func splitTopics(v string) []string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "[")
	v = strings.TrimSuffix(v, "]")
	if v == "" {
		return []string{}
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), `"`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
