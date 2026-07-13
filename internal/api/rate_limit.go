package api

import (
	"net/http"
	"sync"
	"time"
)

// RateLimit defaults (per-hour per-user).
const (
	defaultLimit    = 5000
	defaultWindow   = time.Hour
	anonLimit       = 60
	anonWindow      = time.Hour
	cleanupInterval = 10 * time.Minute
)

type rateBucket struct {
	count   int
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

// Package-level limiters.
var (
	coreLimiter = NewRateLimiter(defaultLimit, defaultWindow)
	anonLimiter = NewRateLimiter(anonLimit, anonWindow)
)

func (s *Server) handleRateLimit(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	isAnon := uid == [16]byte{}
	key := r.RemoteAddr
	if !isAnon {
		key = uid.String()
	}

	var limiter *RateLimiter
	var limit int
	if isAnon {
		limiter = anonLimiter
		limit = anonLimit
	} else {
		limiter = coreLimiter
		limit = defaultLimit
	}

	used, remaining, resetUnix := limiter.AllowIfPossible(key)
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
