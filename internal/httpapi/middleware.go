package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gonews/gonews/internal/auth"
	"github.com/gonews/gonews/internal/metrics"
	"github.com/gonews/gonews/internal/model"
	"github.com/gonews/gonews/internal/platform"
)

type ctxKey int

const userKey ctxKey = 1

func currentUser(ctx context.Context) *model.User {
	u, _ := ctx.Value(userKey).(*model.User)
	return u
}

func currentUserID(ctx context.Context) string {
	if u := currentUser(ctx); u != nil {
		return u.ID
	}
	return ""
}

func (a *API) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 100 {
			id = platform.NewID("req")
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func (a *API) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				a.log.Error("panic recovered", "panic", rec, "path", r.URL.Path, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (a *API) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		metrics.HTTPInFlight.Inc()
		defer metrics.HTTPInFlight.Dec()
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		metrics.HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(sw.status)).Inc()
		metrics.HTTPDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		metrics.HTTPResponseBytes.WithLabelValues(route).Add(float64(sw.bytes))
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && r.URL.Path != "/metrics" {
			a.log.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"ip", clientIP(r),
				"request_id", w.Header().Get("X-Request-ID"),
			)
		}
	})
}

func (a *API) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	allowAll := false
	for _, o := range a.cfg.CORSOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAll || allowed[origin]) {
			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Accept-Language, X-Admin-Token, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Cache, X-RateLimit-Limit, X-RateLimit-Remaining")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) bodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := a.authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired access token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (a *API) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := a.authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (a *API) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := strings.TrimSpace(r.Header.Get("X-Admin-Token")); token != "" && a.cfg.AdminToken != "" {
			if subtle.ConstantTimeCompare([]byte(token), []byte(a.cfg.AdminToken)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		u, err := a.authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "admin authentication required")
			return
		}
		if u.Role != "admin" {
			writeError(w, http.StatusForbidden, "forbidden", "administrator role required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (a *API) authenticate(r *http.Request) (*model.User, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return nil, auth.ErrUnauthorized
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, auth.ErrUnauthorized
	}
	claims, err := a.auth.ParseAccess(parts[1])
	if err != nil {
		return nil, err
	}
	u, err := a.st.GetUserByID(r.Context(), claims.Subject)
	if err != nil {
		return nil, auth.ErrUnauthorized
	}
	return u, nil
}

// --- rate limiting ---

type rateBucket struct {
	count int
	reset time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	limit   int
	window  time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{buckets: map[string]*rateBucket{}, limit: limit, window: window}
}

func (rl *rateLimiter) allow(key string) (bool, int) {
	if rl == nil || rl.limit <= 0 {
		return true, 0
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	if len(rl.buckets) > 10000 {
		for k, b := range rl.buckets {
			if now.After(b.reset) {
				delete(rl.buckets, k)
			}
		}
	}
	b, ok := rl.buckets[key]
	if !ok || now.After(b.reset) {
		rl.buckets[key] = &rateBucket{count: 1, reset: now.Add(rl.window)}
		return true, rl.limit - 1
	}
	if b.count >= rl.limit {
		return false, 0
	}
	b.count++
	return true, rl.limit - b.count
}

func (a *API) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, remaining := a.allow(r, a.limiter, "global:"+clientIP(r), a.limiter.limit)
		w.Header().Set("X-RateLimit-Limit", itoa(a.limiter.limit))
		w.Header().Set("X-RateLimit-Remaining", itoa(remaining))
		if !ok {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests, try again later")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) authRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, remaining := a.allow(r, a.authLimiter, "auth:"+clientIP(r), 20)
		w.Header().Set("X-RateLimit-Remaining", itoa(remaining))
		if !ok {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many authentication attempts")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allow uses Redis when available (shared across replicas) and falls back to
// the in-process limiter if Redis is unavailable.
func (a *API) allow(r *http.Request, local *rateLimiter, key string, limit int) (bool, int) {
	if limit <= 0 {
		return true, 0
	}
	if a.cache != nil && a.cache.Enabled() {
		n, err := a.cache.Incr(r.Context(), key, time.Minute)
		if err == nil {
			return n <= int64(limit), limit - int(n)
		}
	}
	return local.allow(key)
}

// --- response cache ---

type cacheRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newCacheRecorder() *cacheRecorder {
	return &cacheRecorder{header: make(http.Header)}
}

func (c *cacheRecorder) Header() http.Header { return c.header }

func (c *cacheRecorder) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *cacheRecorder) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(b)
}

func (a *API) cacheMiddleware(next http.Handler) http.Handler {
	if a.cache == nil || !a.cache.Enabled() || a.cache.TTL() <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cacheableRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		key := "http:" + r.URL.RequestURI() + "|" + r.Header.Get("Accept-Language")
		if raw, ok := a.cache.GetBytes(r.Context(), key); ok {
			if status, ctype, body, ok := decodeCached(raw); ok {
				if ctype != "" {
					w.Header().Set("Content-Type", ctype)
				}
				w.Header().Set("X-Cache", "HIT")
				w.WriteHeader(status)
				_, _ = w.Write(body)
				return
			}
		}

		rec := newCacheRecorder()
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		for k, values := range rec.header {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
		if rec.status == http.StatusOK && rec.body.Len() > 0 && rec.body.Len() < 1<<20 {
			a.cache.SetBytes(r.Context(), key, encodeCached(rec.status, rec.header.Get("Content-Type"), rec.body.Bytes()), 0)
			w.Header().Set("X-Cache", "MISS")
		} else {
			w.Header().Set("X-Cache", "BYPASS")
		}
		w.WriteHeader(rec.status)
		_, _ = w.Write(rec.body.Bytes())
	})
}

func cacheableRequest(r *http.Request) bool {
	if r.Method != http.MethodGet || strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		return false
	}
	p := r.URL.Path
	switch p {
	case "/api/v1/news", "/api/v1/news/trending", "/api/v1/search",
		"/api/v1/categories", "/api/v1/tags", "/api/v1/sources":
		return true
	}
	if strings.HasPrefix(p, "/api/v1/news/") {
		rest := strings.TrimPrefix(p, "/api/v1/news/")
		return rest != "" && !strings.Contains(rest, "/")
	}
	if strings.HasPrefix(p, "/api/v1/tags/") {
		rest := strings.TrimPrefix(p, "/api/v1/tags/")
		return rest != "" && !strings.Contains(rest, "/")
	}
	return false
}

func encodeCached(status int, contentType string, body []byte) []byte {
	out := make([]byte, 0, len(body)+64)
	out = strconv.AppendInt(out, int64(status), 10)
	out = append(out, '\n')
	out = append(out, contentType...)
	out = append(out, '\n')
	out = append(out, body...)
	return out
}

func decodeCached(raw []byte) (int, string, []byte, bool) {
	i := bytes.IndexByte(raw, '\n')
	if i < 0 {
		return 0, "", nil, false
	}
	j := bytes.IndexByte(raw[i+1:], '\n')
	if j < 0 {
		return 0, "", nil, false
	}
	status, err := strconv.Atoi(string(raw[:i]))
	if err != nil {
		return 0, "", nil, false
	}
	return status, string(raw[i+1 : i+1+j]), raw[i+1+j+1:], true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func itoa(n int) string {
	if n < 0 {
		n = 0
	}
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
