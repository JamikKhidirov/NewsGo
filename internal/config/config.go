// Package config loads all runtime settings from environment variables.
// A local .env file is loaded automatically when present (existing env wins).
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full application configuration.
type Config struct {
	Env           string
	LogLevel      string
	HTTPAddr      string
	PublicBaseURL string

	DatabaseURL string

	RedisURL    string
	CacheTTL    time.Duration
	CachePrefix string

	JWTSecret   string
	AccessTTL   time.Duration
	RefreshTTL  time.Duration
	AdminToken  string
	AdminEmails map[string]bool

	CORSOrigins        []string
	RateLimitPerMinute int

	IngestOnStart  bool
	IngestInterval time.Duration
	IngestTimeout  time.Duration
	Providers      []string // empty = all registered providers
	ProviderLimit  int
	UserAgent      string
	RetentionDays  int

	TranslateEnabled   bool
	TranslateProviders []string
	TranslateTargets   []string
	TranslateWorkers   int
	TranslateTimeout   time.Duration
	TranslateMaxChars  int
	LibreTranslateURL  string
	LingvaURL          string

	KafkaBrokers   []string
	KafkaEnabled   bool
	KafkaPushTopic string
	KafkaNewsTopic string
	KafkaGroup     string

	PushEnabled           bool
	PushWorkers           int
	FCMProjectID          string
	FCMServiceAccountFile string
	APNSKeyFile           string
	APNSKeyID             string
	APNSTeamID            string
	APNSTopic             string
	APNSSandbox           bool
	PushBroadcast         bool
	PushInterval          time.Duration
	PushMinScore          float64
}

// Load reads configuration from the environment, applying safe defaults.
func Load() (*Config, error) {
	_ = loadDotEnv(".env")

	cfg := &Config{
		Env:           env("GONEWS_ENV", "development"),
		LogLevel:      env("GONEWS_LOG_LEVEL", "info"),
		HTTPAddr:      env("GONEWS_HTTP_ADDR", ":8080"),
		PublicBaseURL: env("GONEWS_PUBLIC_BASE_URL", "http://localhost:8080"),
		DatabaseURL:   env("GONEWS_DATABASE_URL", "postgres://gonews:gonews@127.0.0.1:5432/gonews?sslmode=disable"),
		RedisURL:      env("GONEWS_REDIS_URL", ""),
		CacheTTL:      envDuration("GONEWS_CACHE_TTL", 30*time.Second),
		CachePrefix:   env("GONEWS_CACHE_PREFIX", "gonews"),

		JWTSecret:   env("GONEWS_JWT_SECRET", "dev-insecure-secret-change-me"),
		AccessTTL:   envDuration("GONEWS_ACCESS_TTL", 30*time.Minute),
		RefreshTTL:  envDuration("GONEWS_REFRESH_TTL", 30*24*time.Hour),
		AdminToken:  env("GONEWS_ADMIN_TOKEN", ""),
		AdminEmails: toSet(splitList(env("GONEWS_ADMIN_EMAILS", ""))),

		CORSOrigins:        splitList(env("GONEWS_CORS_ORIGINS", "*")),
		RateLimitPerMinute: envInt("GONEWS_RATE_LIMIT_PER_MINUTE", 180),

		IngestOnStart:  envBool("GONEWS_INGEST_ON_START", true),
		IngestInterval: envDuration("GONEWS_INGEST_INTERVAL", 10*time.Minute),
		IngestTimeout:  envDuration("GONEWS_INGEST_TIMEOUT", 90*time.Second),
		Providers:      splitList(env("GONEWS_PROVIDERS", "")),
		ProviderLimit:  envInt("GONEWS_PROVIDER_LIMIT", 25),
		UserAgent:      env("GONEWS_USER_AGENT", "GoNewsBot/1.0 (+https://github.com/gonews/gonews)"),
		RetentionDays:  envInt("GONEWS_RETENTION_DAYS", 90),

		TranslateEnabled:   envBool("GONEWS_TRANSLATE_ENABLED", true),
		TranslateProviders: splitList(env("GONEWS_TRANSLATE_PROVIDERS", "google,lingva,mymemory,libretranslate")),
		TranslateTargets:   splitList(env("GONEWS_TRANSLATE_TARGETS", "ru")),
		TranslateWorkers:   envInt("GONEWS_TRANSLATE_WORKERS", 2),
		TranslateTimeout:   envDuration("GONEWS_TRANSLATE_TIMEOUT", 12*time.Second),
		TranslateMaxChars:  envInt("GONEWS_TRANSLATE_MAX_CHARS", 4500),
		LibreTranslateURL:  env("GONEWS_LIBRETRANSLATE_URL", "https://libretranslate.com"),
		LingvaURL:          env("GONEWS_LINGVA_URL", "https://lingva.ml"),

		KafkaBrokers:   splitList(env("GONEWS_KAFKA_BROKERS", "")),
		KafkaPushTopic: env("GONEWS_KAFKA_PUSH_TOPIC", "gonews.notifications"),
		KafkaNewsTopic: env("GONEWS_KAFKA_NEWS_TOPIC", "gonews.news"),
		KafkaGroup:     env("GONEWS_KAFKA_GROUP", "gonews-push"),

		PushEnabled:           envBool("GONEWS_PUSH_ENABLED", true),
		PushWorkers:           envInt("GONEWS_PUSH_WORKERS", 4),
		FCMProjectID:          env("GONEWS_FCM_PROJECT_ID", ""),
		FCMServiceAccountFile: env("GONEWS_FCM_SERVICE_ACCOUNT_FILE", ""),
		APNSKeyFile:           env("GONEWS_APNS_KEY_FILE", ""),
		APNSKeyID:             env("GONEWS_APNS_KEY_ID", ""),
		APNSTeamID:            env("GONEWS_APNS_TEAM_ID", ""),
		APNSTopic:             env("GONEWS_APNS_TOPIC", "com.gonews.app"),
		APNSSandbox:           envBool("GONEWS_APNS_SANDBOX", false),
		PushBroadcast:         envBool("GONEWS_PUSH_BROADCAST", true),
		PushInterval:          envDuration("GONEWS_PUSH_INTERVAL", 30*time.Minute),
		PushMinScore:          envFloat("GONEWS_PUSH_MIN_SCORE", 20),
	}

	if cfg.TranslateWorkers < 1 {
		cfg.TranslateWorkers = 1
	}
	if cfg.PushWorkers < 1 {
		cfg.PushWorkers = 1
	}
	if cfg.CacheTTL < 0 {
		cfg.CacheTTL = 0
	}
	cfg.KafkaEnabled = len(cfg.KafkaBrokers) > 0
	if cfg.ProviderLimit < 1 || cfg.ProviderLimit > 100 {
		cfg.ProviderLimit = 25
	}
	if cfg.RateLimitPerMinute < 0 {
		cfg.RateLimitPerMinute = 0
	}
	if cfg.RetentionDays < 0 {
		cfg.RetentionDays = 0
	}
	if cfg.Env == "production" && cfg.JWTSecret == "dev-insecure-secret-change-me" {
		return nil, fmt.Errorf("GONEWS_JWT_SECRET must be set in production")
	}
	return cfg, nil
}

// IsAdminEmail reports whether the email is configured as an administrator.
func (c *Config) IsAdminEmail(email string) bool {
	return c.AdminEmails[strings.ToLower(strings.TrimSpace(email))]
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

func envFloat(key string, def float64) float64 {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return def
	}
	return f
}

func envDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return d
}

func splitList(v string) []string {
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[strings.ToLower(strings.TrimSpace(item))] = true
	}
	return m
}

// loadDotEnv is a tiny .env parser: KEY=VALUE, # comments, optional quotes.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
