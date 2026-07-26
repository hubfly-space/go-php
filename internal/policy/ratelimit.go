package policy

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements token bucket rate limiting.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*Bucket
	rate    int
	burst   int
}

// Bucket is a single token bucket.
type Bucket struct {
	Tokens    float64
	MaxTokens float64
	RefillRate float64 // tokens per second
	LastCheck time.Time
}

// NewRateLimiter creates a rate limiter.
func NewRateLimiter(ratePerMinute, burst int) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*Bucket),
		rate:    ratePerMinute,
		burst:   burst,
	}
}

// Allow checks if a request is allowed for the given key.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		maxTokens := float64(rl.burst)
		if maxTokens < 1 {
			maxTokens = float64(rl.rate)
		}
		b = &Bucket{
			Tokens:    maxTokens,
			MaxTokens: maxTokens,
			RefillRate: float64(rl.rate) / 60.0,
			LastCheck: time.Now(),
		}
		rl.buckets[key] = b
	}

	now := time.Now()
	elapsed := now.Sub(b.LastCheck).Seconds()
	b.Tokens += elapsed * b.RefillRate
	if b.Tokens > b.MaxTokens {
		b.Tokens = b.MaxTokens
	}
	b.LastCheck = now

	if b.Tokens < 1 {
		return false
	}

	b.Tokens--
	return true
}

// Cleanup removes old buckets.
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-5 * time.Minute)
	for k, b := range rl.buckets {
		if b.LastCheck.Before(cutoff) {
			delete(rl.buckets, k)
		}
	}
}

// BucketStatus returns the current state of a bucket.
func (rl *RateLimiter) BucketStatus(key string) (tokens float64, ok bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		return 0, false
	}
	return b.Tokens, true
}

// PerRouteLimiter manages rate limiters per route.
type PerRouteLimiter struct {
	limiters map[string]*RateLimiter
	global   *RateLimiter
	mu       sync.RWMutex
}

// NewPerRouteLimiter creates a per-route rate limiter.
func NewPerRouteLimiter(globalRate int) *PerRouteLimiter {
	return &PerRouteLimiter{
		limiters: make(map[string]*RateLimiter),
		global:   NewRateLimiter(globalRate, globalRate),
	}
}

// SetRoute sets a rate limit for a specific route.
func (prl *PerRouteLimiter) SetRoute(route string, ratePerMinute, burst int) {
	prl.mu.Lock()
	defer prl.mu.Unlock()

	prl.limiters[route] = NewRateLimiter(ratePerMinute, burst)
}

// Allow checks both global and route-specific rate limits.
func (prl *PerRouteLimiter) Allow(route, clientIP string) bool {
	// Global limit.
	if !prl.global.Allow(clientIP) {
		return false
	}

	// Route-specific limit.
	prl.mu.RLock()
	limiter, ok := prl.limiters[route]
	prl.mu.RUnlock()

	if ok {
		return limiter.Allow(route + ":" + clientIP)
	}

	return true
}

// Middleware returns an HTTP middleware that rate-limits requests.
func (prl *PerRouteLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		clientIP := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			clientIP = xff
		}

		if !prl.Allow(key, clientIP) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
