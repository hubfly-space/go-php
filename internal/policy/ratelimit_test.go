package policy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(5, 5)

	for i := 0; i < 5; i++ {
		if !rl.Allow("test") {
			t.Errorf("request %d should be allowed", i)
		}
	}

	if rl.Allow("test") {
		t.Error("6th request should be rate limited")
	}
}

func TestRateLimiterSeparateKeys(t *testing.T) {
	rl := NewRateLimiter(2, 2)

	if !rl.Allow("a") {
		t.Error("a should be allowed")
	}
	if !rl.Allow("a") {
		t.Error("a second should be allowed")
	}
	if rl.Allow("a") {
		t.Error("a third should be limited")
	}

	if !rl.Allow("b") {
		t.Error("b should be allowed independently")
	}
}

func TestPerRouteLimiter(t *testing.T) {
	prl := NewPerRouteLimiter(100)
	prl.SetRoute("/api/upload", 2, 2)

	for i := 0; i < 2; i++ {
		if !prl.Allow("/api/upload", "1.2.3.4") {
			t.Errorf("upload request %d should be allowed", i)
		}
	}

	if prl.Allow("/api/upload", "1.2.3.4") {
		t.Error("upload 3rd should be limited")
	}

	// Different route should be fine.
	if !prl.Allow("/api/status", "1.2.3.4") {
		t.Error("status should be allowed")
	}
}

func TestPerRouteLimiterMiddleware(t *testing.T) {
	prl := NewPerRouteLimiter(100)
	prl.SetRoute("/test", 1, 1)

	handler := prl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("first request: status = %d, want 200", w.Code)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 429 {
		t.Errorf("second request: status = %d, want 429", w.Code)
	}
}

func TestBucketStatus(t *testing.T) {
	rl := NewRateLimiter(10, 10)

	_, ok := rl.BucketStatus("nonexistent")
	if ok {
		t.Error("expected no bucket for nonexistent key")
	}

	rl.Allow("key")
	tokens, ok := rl.BucketStatus("key")
	if !ok {
		t.Fatal("expected bucket for key")
	}
	if tokens < 8 || tokens > 10 {
		t.Errorf("tokens = %v, expected ~9", tokens)
	}
}

func TestClientKeyIgnoresForwardedHeader(t *testing.T) {
	// Trusting X-Forwarded-For without a trusted-proxy list (§10.3) would let
	// any client mint a fresh bucket per request and bypass the limit entirely.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.10:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.99")

	if got := ClientKey(req); got != "192.0.2.10" {
		t.Errorf("ClientKey = %q, want the peer address %q", got, "192.0.2.10")
	}
}

func TestClientKeyStripsPort(t *testing.T) {
	// Keeping the port would give every new connection from one client its own
	// bucket, which defeats the limit.
	req := httptest.NewRequest("GET", "/", nil)

	req.RemoteAddr = "192.0.2.10:1111"
	first := ClientKey(req)
	req.RemoteAddr = "192.0.2.10:2222"
	second := ClientKey(req)

	if first != second {
		t.Errorf("ClientKey varies by port: %q vs %q", first, second)
	}
}

func TestRateLimiterMiddlewareBlocksRepeatOffender(t *testing.T) {
	prl := NewPerRouteLimiter(60)
	prl.SetRoute("/api", 2, 2)

	handler := prl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	newReq := func() *http.Request {
		r := httptest.NewRequest("GET", "/api", nil)
		r.RemoteAddr = "192.0.2.10:1234"
		// Varying the header must not help.
		r.Header.Set("X-Forwarded-For", "203.0.113.1")
		return r
	}

	var last int
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, newReq())
		last = w.Code
		if last == http.StatusTooManyRequests {
			if w.Header().Get("Retry-After") == "" {
				t.Error("429 response is missing Retry-After")
			}
			return
		}
	}
	t.Errorf("never rate limited after 10 requests (last status %d)", last)
}

func TestRateLimiterBucketMapIsBounded(t *testing.T) {
	rl := NewRateLimiter(60, 60)

	for i := 0; i < maxBuckets+1000; i++ {
		rl.Allow(fmt.Sprintf("client-%d", i))
	}

	if got := rl.Buckets(); got > maxBuckets {
		t.Errorf("buckets = %d, want <= %d (§24.3 forbids unbounded client-key maps)", got, maxBuckets)
	}
}

func TestPerRouteLimiterStartCleanupExitsOnCancel(t *testing.T) {
	prl := NewPerRouteLimiter(60)

	ctx, cancel := context.WithCancel(context.Background())
	done := prl.StartCleanup(ctx, time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup goroutine did not exit after context cancellation")
	}
}
