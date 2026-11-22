package httpapi

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/gonews/gonews/internal/auth"
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
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		a.metrics.Observe(sw.status)
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
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-RateLimit-Limit, X-RateLimit-Remaining")
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
		ok, remaining := a.limiter.allow(clientIP(r))
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
		ok, remaining := a.authLimiter.allow(clientIP(r))
		w.Header().Set("X-RateLimit-Remaining", itoa(remaining))
		if !ok {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many authentication attempts")
			return
		}
		next.ServeHTTP(w, r)
	})
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
