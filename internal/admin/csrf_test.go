package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCSRFProtect_GenerateAndValidate(t *testing.T) {
	csrf := NewCSRFProtect("test-secret-key", 1*time.Hour)

	token := csrf.GenerateToken()
	if token == "" {
		t.Error("expected non-empty token")
	}

	if !csrf.Validate(token) {
		t.Error("expected valid token")
	}
}

func TestCSRFProtect_Consume(t *testing.T) {
	csrf := NewCSRFProtect("test-secret-key", 1*time.Hour)

	token := csrf.GenerateToken()

	// First consume should succeed.
	if !csrf.Consume(token) {
		t.Error("expected first consume to succeed")
	}

	// Second consume should fail (single-use).
	if csrf.Consume(token) {
		t.Error("expected second consume to fail")
	}
}

func TestCSRFProtect_ExpiredToken(t *testing.T) {
	csrf := NewCSRFProtect("test-secret-key", 1*time.Millisecond)

	token := csrf.GenerateToken()
	time.Sleep(10 * time.Millisecond)

	if csrf.Validate(token) {
		t.Error("expected expired token to be invalid")
	}
}

func TestCSRFProtect_EmptyToken(t *testing.T) {
	csrf := NewCSRFProtect("test-secret-key", 1*time.Hour)

	if csrf.Validate("") {
		t.Error("expected empty token to be invalid")
	}
}

func TestCSRFMiddleware_SkipsGET(t *testing.T) {
	csrf := NewCSRFProtect("test-secret", 1*time.Hour)

	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200 for GET, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_RejectsPOSTWithoutToken(t *testing.T) {
	csrf := NewCSRFProtect("test-secret", 1*time.Hour)

	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("POST", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != 403 {
		t.Errorf("expected 403 for POST without token, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_AcceptsValidToken(t *testing.T) {
	csrf := NewCSRFProtect("test-secret", 1*time.Hour)
	token := csrf.GenerateToken()

	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("POST", "/api/test", nil)
	req.Header.Set("X-CSRF-Token", token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200 with valid token, got %d", rec.Code)
	}
}

func TestSecureHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	SecureHeaders(rec)

	headers := rec.Header()
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected nosniff")
	}
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Error("expected DENY")
	}
}

func TestValidateOrigin(t *testing.T) {
	allowed := []string{"https://example.com", "https://admin.example.com"}

	tests := []struct {
		origin string
		valid  bool
	}{
		{"https://example.com", true},
		{"https://admin.example.com", true},
		{"https://evil.com", false},
		{"", true}, // non-browser
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		if tt.origin != "" {
			req.Header.Set("Origin", tt.origin)
		}

		result := ValidateOrigin(req, allowed)
		if result != tt.valid {
			t.Errorf("origin=%q: expected %v, got %v", tt.origin, tt.valid, result)
		}
	}
}

func TestIsBrowserRequest(t *testing.T) {
	tests := []struct {
		ua       string
		expected bool
	}{
		{"Mozilla/5.0 (X11; Linux x86_64)", true},
		{"curl/7.68.0", false},
		{"", false},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("User-Agent", tt.ua)

		result := IsBrowserRequest(req)
		if result != tt.expected {
			t.Errorf("ua=%q: expected %v, got %v", tt.ua, tt.expected, result)
		}
	}
}

func TestGenerateSecret(t *testing.T) {
	secret := GenerateSecret(32)
	if len(secret) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("expected 64 hex chars, got %d", len(secret))
	}

	// Two secrets should be different.
	secret2 := GenerateSecret(32)
	if secret == secret2 {
		t.Error("expected different secrets")
	}
}

func TestSecurityMiddleware(t *testing.T) {
	middleware := SecurityMiddleware("test-secret", []string{"https://example.com"})

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	// Check security headers.
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected security headers")
	}
}

func TestSecurityMiddleware_RejectsBadOrigin(t *testing.T) {
	middleware := SecurityMiddleware("test-secret", []string{"https://example.com"})

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("POST", "/api/test", strings.NewReader("data"))
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != 403 {
		t.Errorf("expected 403 for bad origin, got %d", rec.Code)
	}
}
