-- GoNews initial schema (PostgreSQL 16+).
-- Timestamps are TIMESTAMPTZ; all application code stores UTC.

CREATE TABLE IF NOT EXISTS sources (
    id         TEXT PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    homepage   TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL DEFAULT 'rss',
    lang       TEXT NOT NULL DEFAULT 'en',
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS articles (
    id             TEXT PRIMARY KEY,
    source_id      TEXT NOT NULL REFERENCES sources(id),
    external_id    TEXT NOT NULL DEFAULT '',
    url            TEXT NOT NULL,
    canonical_url  TEXT NOT NULL,
    title          TEXT NOT NULL,
    summary        TEXT NOT NULL DEFAULT '',
    content        TEXT NOT NULL DEFAULT '',
    image_url      TEXT NOT NULL DEFAULT '',
    author         TEXT NOT NULL DEFAULT '',
    lang           TEXT NOT NULL DEFAULT 'en',
    category       TEXT NOT NULL DEFAULT 'world',
    published_at   TIMESTAMPTZ NOT NULL,
    fetched_at     TIMESTAMPTZ NOT NULL,
    hot_score      DOUBLE PRECISION NOT NULL DEFAULT 0,
    likes_count    INTEGER NOT NULL DEFAULT 0,
    comments_count INTEGER NOT NULL DEFAULT 0,
    views_count    INTEGER NOT NULL DEFAULT 0,
    unique_hash    TEXT NOT NULL UNIQUE
);
CREATE INDEX IF NOT EXISTS idx_articles_published ON articles(published_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_source ON articles(source_id, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_category ON articles(category, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_hot ON articles(hot_score DESC);
CREATE INDEX IF NOT EXISTS idx_articles_lang_published ON articles(lang, published_at DESC);

CREATE TABLE IF NOT EXISTS article_translations (
    article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    lang       TEXT NOT NULL,
    title      TEXT NOT NULL,
    summary    TEXT NOT NULL DEFAULT '',
    content    TEXT NOT NULL DEFAULT '',
    provider   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (article_id, lang)
);

CREATE TABLE IF NOT EXISTS translation_jobs (
    id              BIGSERIAL PRIMARY KEY,
    article_id      TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    target_lang     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    next_attempt_at TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    UNIQUE (article_id, target_lang)
);
CREATE INDEX IF NOT EXISTS idx_translation_jobs_pending ON translation_jobs(status, next_attempt_at);

CREATE TABLE IF NOT EXISTS tags (
    id         TEXT PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE,
    name_ru    TEXT NOT NULL,
    name_en    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS article_tags (
    article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    tag_id     TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (article_id, tag_id)
);
CREATE INDEX IF NOT EXISTS idx_article_tags_tag ON article_tags(tag_id);

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user',
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    user_agent TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

CREATE TABLE IF NOT EXISTS likes (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, article_id)
);
CREATE INDEX IF NOT EXISTS idx_likes_article ON likes(article_id);

CREATE TABLE IF NOT EXISTS bookmarks (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, article_id)
);
CREATE INDEX IF NOT EXISTS idx_bookmarks_user ON bookmarks(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS follows (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tag_id     TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, tag_id)
);

CREATE TABLE IF NOT EXISTS comments (
    id         TEXT PRIMARY KEY,
    article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_id  TEXT REFERENCES comments(id) ON DELETE CASCADE,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_comments_article ON comments(article_id, created_at DESC);

CREATE TABLE IF NOT EXISTS article_views (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    viewed_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, article_id)
);
CREATE INDEX IF NOT EXISTS idx_article_views_user ON article_views(user_id, viewed_at DESC);

CREATE TABLE IF NOT EXISTS devices (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token        TEXT NOT NULL UNIQUE,
    platform     TEXT NOT NULL DEFAULT 'android',
    locale       TEXT NOT NULL DEFAULT 'ru',
    topics       TEXT NOT NULL DEFAULT '[]',
    enabled      INTEGER NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id);
CREATE INDEX IF NOT EXISTS idx_devices_platform ON devices(platform, enabled);

CREATE TABLE IF NOT EXISTS notifications (
    id         TEXT PRIMARY KEY,
    user_id    TEXT REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL DEFAULT 'broadcast',
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    data       TEXT NOT NULL DEFAULT '',
    article_id TEXT REFERENCES articles(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notifications_created ON notifications(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_reads (
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    read_at         TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, notification_id)
);

CREATE TABLE IF NOT EXISTS ingest_runs (
    id          BIGSERIAL PRIMARY KEY,
    started_at  TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    status      TEXT NOT NULL DEFAULT 'running',
    fetched     INTEGER NOT NULL DEFAULT 0,
    inserted    INTEGER NOT NULL DEFAULT 0,
    updated     INTEGER NOT NULL DEFAULT 0,
    errors      INTEGER NOT NULL DEFAULT 0,
    details     TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
