package policy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// maxBuckets caps how many distinct client keys a limiter tracks. §24.3: "Do
// not use unbounded client-key maps." Once the cap is hit, new keys are refused
// admission rather than allocated — an attacker spraying keys degrades into
// being rate limited, which is the safe direction.
const maxBuckets = 100_000

// RateLimiter implements token bucket rate limiting.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*Bucket
	rate    int
	burst   int
}

// Bucket is a single token bucket.
type Bucket struct {
	Tokens     float64
	MaxTokens  float64
	RefillRate float64 // tokens per second
	LastCheck  time.Time
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
		if len(rl.buckets) >= maxBuckets {
			// Refuse rather than grow. Allocating here is what turns a key
			// spray into a memory exhaustion primitive.
			return false
		}
		maxTokens := float64(rl.burst)
		if maxTokens < 1 {
			maxTokens = float64(rl.rate)
		}
		b = &Bucket{
			Tokens:     maxTokens,
			MaxTokens:  maxTokens,
			RefillRate: float64(rl.rate) / 60.0,
			LastCheck:  time.Now(),
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

// Buckets returns the number of tracked client keys.
func (rl *RateLimiter) Buckets() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.buckets)
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

// Cleanup evicts stale buckets from the global limiter and every route limiter.
func (prl *PerRouteLimiter) Cleanup() {
	prl.global.Cleanup()

	prl.mu.RLock()
	limiters := make([]*RateLimiter, 0, len(prl.limiters))
	for _, l := range prl.limiters {
		limiters = append(limiters, l)
	}
	prl.mu.RUnlock()

	for _, l := range limiters {
		l.Cleanup()
	}
}

// StartCleanup evicts stale buckets on an interval until ctx is canceled, and
// returns a channel closed when the goroutine has exited.
//
// §24.3 requires bounded client-key state. Without this loop the bucket map
// only ever grows, so wiring the limiter without calling this trades a rate
// limit for a memory leak.
func (prl *PerRouteLimiter) StartCleanup(ctx context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer func() {
			_ = recover()
		}()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				prl.Cleanup()
			}
		}
	}()

	return done
}

// ClientKey derives the rate limiting key for a request.
//
// It deliberately uses the transport peer address and ignores X-Forwarded-For.
// Trusting that header without validating the peer against a trusted-proxy list
// (§10.3, not yet implemented) would let any client mint a fresh bucket per
// request by varying the header, which removes the rate limit entirely.
//
// The port is stripped, because otherwise every new connection from one client
// gets its own bucket.
func ClientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Middleware returns an HTTP middleware that rate-limits requests.
func (prl *PerRouteLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !prl.Allow(r.URL.Path, ClientKey(r)) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limited"}`)
			return
		}

		next.ServeHTTP(w, r)
	})
}
