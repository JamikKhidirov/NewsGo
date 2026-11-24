// Package cache provides a Redis-backed response cache, rate limiter and
// health probe. When Redis is not configured the cache is a transparent no-op,
// so the service still runs in single-node development mode.
package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/gonews/gonews/internal/metrics"
	"github.com/redis/go-redis/v9"
)

// Cache is a thin Redis facade.
type Cache struct {
	rdb    *redis.Client
	ttl    time.Duration
	prefix string
	log    *slog.Logger
}

// New connects to Redis. Connection failures are logged and degrade the cache
// to a no-op instead of failing the whole application.
func New(rawURL string, ttl time.Duration, prefix string, log *slog.Logger) *Cache {
	if log == nil {
		log = slog.Default()
	}
	if prefix == "" {
		prefix = "gonews"
	}
	c := &Cache{ttl: ttl, prefix: prefix, log: log}
	if strings.TrimSpace(rawURL) == "" {
		log.Info("redis is not configured; response cache and distributed rate limiting are disabled")
		return c
	}
	opt, err := redis.ParseURL(rawURL)
	if err != nil {
		log.Warn("invalid GONEWS_REDIS_URL; cache disabled", "error", err)
		return c
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Warn("redis is unreachable; cache disabled", "error", err)
		client.Close()
		return c
	}
	c.rdb = client
	log.Info("redis connected", "addr", opt.Addr, "db", opt.DB)
	return c
}

// Enabled reports whether Redis is available.
func (c *Cache) Enabled() bool { return c != nil && c.rdb != nil }

// TTL returns the configured response cache lifetime.
func (c *Cache) TTL() time.Duration {
	if c == nil || c.ttl <= 0 {
		return 0
	}
	return c.ttl
}

func (c *Cache) key(parts ...string) string {
	return c.prefix + ":" + strings.Join(parts, ":")
}

// GetJSON loads a JSON value into dst and reports whether it was found.
func (c *Cache) GetJSON(ctx context.Context, key string, dst any) bool {
	if !c.Enabled() {
		return false
	}
	raw, err := c.rdb.Get(ctx, c.key(key)).Bytes()
	if err != nil {
		if err != redis.Nil {
			metrics.CacheOps.WithLabelValues("get", "error").Inc()
			c.log.Debug("cache get failed", "key", key, "error", err)
		}
		metrics.CacheOps.WithLabelValues("get", "miss").Inc()
		return false
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		metrics.CacheOps.WithLabelValues("get", "error").Inc()
		return false
	}
	metrics.CacheOps.WithLabelValues("get", "hit").Inc()
	return true
}

// GetBytes loads a raw value.
func (c *Cache) GetBytes(ctx context.Context, key string) ([]byte, bool) {
	if !c.Enabled() {
		return nil, false
	}
	raw, err := c.rdb.Get(ctx, c.key(key)).Bytes()
	if err != nil {
		metrics.CacheOps.WithLabelValues("get", "miss").Inc()
		return nil, false
	}
	metrics.CacheOps.WithLabelValues("get", "hit").Inc()
	return raw, true
}

// SetBytes stores a raw value with the given TTL (0 uses the default TTL).
func (c *Cache) SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) {
	if !c.Enabled() {
		return
	}
	if ttl <= 0 {
		ttl = c.ttl
	}
	if ttl <= 0 {
		return
	}
	if err := c.rdb.Set(ctx, c.key(key), value, ttl).Err(); err != nil {
		metrics.CacheOps.WithLabelValues("set", "error").Inc()
		return
	}
	metrics.CacheOps.WithLabelValues("set", "ok").Inc()
}

// SetJSON stores a JSON value with the given TTL (0 uses the default TTL).
func (c *Cache) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) {
	if !c.Enabled() {
		return
	}
	if ttl <= 0 {
		ttl = c.ttl
	}
	if ttl <= 0 {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	if err := c.rdb.Set(ctx, c.key(key), raw, ttl).Err(); err != nil {
		metrics.CacheOps.WithLabelValues("set", "error").Inc()
		c.log.Debug("cache set failed", "key", key, "error", err)
		return
	}
	metrics.CacheOps.WithLabelValues("set", "ok").Inc()
}

// DeletePrefix removes every key under the prefix (used after ingestion).
func (c *Cache) DeletePrefix(ctx context.Context, prefix string) {
	if !c.Enabled() {
		return
	}
	match := c.key(prefix) + "*"
	iter := c.rdb.Scan(ctx, 0, match, 500).Iterator()
	batch := make([]string, 0, 500)
	for iter.Next(ctx) {
		batch = append(batch, iter.Val())
		if len(batch) >= 500 {
			if err := c.rdb.Del(ctx, batch...).Err(); err != nil {
				c.log.Debug("cache delete failed", "error", err)
				break
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		_ = c.rdb.Del(ctx, batch...).Err()
	}
	if err := iter.Err(); err != nil {
		c.log.Debug("cache scan failed", "error", err)
	}
	metrics.CacheOps.WithLabelValues("delete", "ok").Inc()
}

// Incr increments a counter and sets the expiry on first use.
func (c *Cache) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	if !c.Enabled() {
		return 0, redis.ErrClosed
	}
	full := c.key("rl", key)
	pipe := c.rdb.TxPipeline()
	incr := pipe.Incr(ctx, full)
	pipe.Expire(ctx, full, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// Healthy checks Redis connectivity.
func (c *Cache) Healthy(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	return c.rdb.Ping(ctx).Err()
}

// Close releases the Redis connection.
func (c *Cache) Close() error {
	if !c.Enabled() {
		return nil
	}
	return c.rdb.Close()
}
