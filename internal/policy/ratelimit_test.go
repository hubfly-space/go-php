package policy

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
