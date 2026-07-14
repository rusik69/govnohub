package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// RateLimit defaults (per-hour per-user).
const (
	defaultLimit    = 5000
	defaultWindow   = time.Hour
	anonLimit       = 60
	anonWindow      = time.Hour
	searchLimit     = 100
	searchWindow    = time.Hour
	cleanupInterval = 10 * time.Minute
)

type rateBucket struct {
	count       int
	windowStart time.Time
}

// RateLimiter is a simple per-key sliding-window rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	limit   int
	window  time.Duration
	stopCh  chan struct{}
}

// NewRateLimiter creates a rate limiter with the given per-window limit.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*rateBucket),
		limit:   limit,
		window:  window,
		stopCh:  make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-rl.window)
			for k, b := range rl.buckets {
				if b.windowStart.Before(cutoff) {
					delete(rl.buckets, k)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

// AllowIfPossible checks whether the given key is allowed under its rate limit.
// It returns the request count so far and the remaining budget.
func (rl *RateLimiter) AllowIfPossible(key string) (used, remaining int, resetUnix int64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &rateBucket{windowStart: now}
		rl.buckets[key] = b
	}

	// If the window expired, reset.
	if now.Sub(b.windowStart) >= rl.window {
		b.count = 0
		b.windowStart = now
	}

	b.count++
	remaining = rl.limit - b.count
	if remaining < 0 {
		remaining = 0
	}
	resetUnix = b.windowStart.Add(rl.window).Unix()
	return b.count, remaining, resetUnix
}

// Stop stops the background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// rateLimitKey returns the rate-limit key for the request:
// authenticated users keyed by user ID, anonymous by remote IP.
func rateLimitKey(r *http.Request, uid uuid.UUID) string {
	if uid != uuid.Nil {
		return "user:" + uid.String()
	}
	// Use X-Forwarded-For or X-Real-IP if available, fall back to RemoteAddr.
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		// Strip port from RemoteAddr.
		ip = r.RemoteAddr
		for i := 0; i < len(ip); i++ {
			if ip[i] == ':' {
				ip = ip[:i]
				break
			}
		}
	}
	return "ip:" + ip
}

// setRateLimitHeaders writes X-RateLimit-* headers on the response.
func setRateLimitHeaders(w http.ResponseWriter, limit, remaining int, resetUnix int64) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetUnix, 10))
}

// RateLimitMiddleware returns a chi middleware that enforces rate limits
// using the provided limiter. It extracts user identity from the context
// (set by the authenticate middleware) or falls back to the remote IP.
// When the limit is exceeded it responds with 429 and the standard
// rate-limit headers.
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid := userIDFrom(r.Context())
			key := rateLimitKey(r, uid)
						_, remaining, resetUnix := limiter.AllowIfPossible(key)

			setRateLimitHeaders(w, limiter.limit, remaining, resetUnix)

			if remaining == 0 {
				// Calculate seconds until reset for Retry-After.
				nowUnix := time.Now().Unix()
				retryAfter := resetUnix - nowUnix
				if retryAfter < 0 {
					retryAfter = 0
				}
				w.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitByMethod returns a middleware group that applies the given
// limiter to requests matching one of the given methods.
func rateLimitByMethod(limiter *RateLimiter, methods ...string) func(http.Handler) http.Handler {
	methodSet := make(map[string]bool, len(methods))
	for _, m := range methods {
		methodSet[m] = true
	}
	inner := RateLimitMiddleware(limiter)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if methodSet[r.Method] {
				inner(next).ServeHTTP(w, r.WithContext(r.Context()))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WithRateLimit is a helper to attach a rate-limited sub-router.
func WithRateLimit(r chi.Router, limiter *RateLimiter) chi.Router {
	r.Use(RateLimitMiddleware(limiter))
	return r
}

// --- Per-endpoint rate-limit status ---

type rateLimitResource struct {
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
	Used      int   `json:"used"`
}

type rateLimitResponse struct {
	Resources map[string]rateLimitResource `json:"resources"`
	Rate      rateLimitResource            `json:"rate"`
}

func (s *Server) handleRateLimit(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	isAnon := uid == uuid.Nil
	key := rateLimitKey(r, uid)

	var limiter *RateLimiter
	var limit int
	if isAnon {
		limiter = s.anonLimiter
		limit = anonLimit
	} else {
		limiter = s.coreLimiter
		limit = defaultLimit
	}

	// AllowIfPossible increments the counter; for the status endpoint we
	// want to show stats without counting the request. We track the count
	// separately here.
	used, remaining, resetUnix := limiter.AllowIfPossible(key)
	// Subtract 1 to show the state *before* this status check.
	if used > 0 {
		used--
		remaining++
		if remaining > limit {
			remaining = limit
		}
	}

	resp := rateLimitResponse{
		Resources: map[string]rateLimitResource{
			"core": {
				Limit:     limit,
				Remaining: remaining,
				Reset:     resetUnix,
				Used:      used,
			},
		},
		Rate: rateLimitResource{
			Limit:     limit,
			Remaining: remaining,
			Reset:     resetUnix,
			Used:      used,
		},
	}
	jsonOK(w, resp)
}
