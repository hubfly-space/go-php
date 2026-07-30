package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CSRFProtect provides CSRF protection for browser-based admin API access.
type CSRFProtect struct {
	mu        sync.RWMutex
	tokens    map[string]time.Time // token -> creation time
	secret    []byte
	ttl       time.Duration
	maxTokens int
}

// NewCSRFProtect creates a CSRF protection manager.
func NewCSRFProtect(secret string, ttl time.Duration) *CSRFProtect {
	return &CSRFProtect{
		tokens:    make(map[string]time.Time),
		secret:    []byte(secret),
		ttl:       ttl,
		maxTokens: 100,
	}
}

// hmacSign computes HMAC-SHA256 over data.
func hmacSign(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// verifySignature checks a "<nonce-hex>.<mac-hex>" token's HMAC in constant
// time and returns the nonce.
//
// This used to be missing entirely: Validate and Consume did a bare map lookup,
// so the signature was decorative. Verifying it means a token that was never
// issued by this process is rejected on its own merits, not merely because it
// is absent from a map that a restart empties.
func (c *CSRFProtect) verifySignature(token string) bool {
	noncePart, macPart, found := strings.Cut(token, ".")
	if !found {
		return false
	}

	nonce, err := hex.DecodeString(noncePart)
	if err != nil {
		return false
	}
	gotMAC, err := hex.DecodeString(macPart)
	if err != nil {
		return false
	}

	return hmac.Equal(gotMAC, hmacSign(c.secret, nonce))
}

// GenerateToken creates a new CSRF token.
func (c *CSRFProtect) GenerateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand cannot fail on supported platforms; if it somehow does,
		// returning a predictable token would be far worse than panicking.
		panic("admin: crypto/rand failed: " + err.Error())
	}

	mac := hmacSign(c.secret, b)
	token := hex.EncodeToString(b) + "." + hex.EncodeToString(mac)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.tokens[token] = time.Now()
	c.evict()

	return token
}

// Validate checks if a CSRF token is valid.
func (c *CSRFProtect) Validate(token string) bool {
	if token == "" || !c.verifySignature(token) {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	created, ok := c.tokens[token]
	if !ok {
		return false
	}

	return time.Since(created) <= c.ttl
}

// Consume validates and removes a token (single-use).
func (c *CSRFProtect) Consume(token string) bool {
	if token == "" || !c.verifySignature(token) {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	created, ok := c.tokens[token]
	if !ok {
		return false
	}

	delete(c.tokens, token)
	return time.Since(created) <= c.ttl
}

func (c *CSRFProtect) evict() {
	if len(c.tokens) <= c.maxTokens {
		return
	}

	cutoff := time.Now().Add(-c.ttl)
	for t, created := range c.tokens {
		if created.Before(cutoff) {
			delete(c.tokens, t)
		}
	}
}

// CSRFMiddleware returns HTTP middleware that validates CSRF tokens.
func (c *CSRFProtect) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip for non-mutating methods.
		if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		// Check X-CSRF-Token header.
		token := r.Header.Get("X-CSRF-Token")
		if token == "" {
			// Check form field.
			r.ParseForm()
			token = r.FormValue("csrf_token")
		}

		if !c.Consume(token) {
			http.Error(w, `{"error":"invalid CSRF token"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ValidateOrigin checks the Origin header against allowed origins.
func ValidateOrigin(r *http.Request, allowedOrigins []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Non-browser request.
	}

	for _, allowed := range allowedOrigins {
		if subtle.ConstantTimeCompare([]byte(origin), []byte(allowed)) == 1 {
			return true
		}
	}

	return false
}

// GenerateSecret generates a random hex-encoded secret.
func GenerateSecret(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// IsBrowserRequest detects if the request is from a browser.
func IsBrowserRequest(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	return strings.Contains(ua, "mozilla") || strings.Contains(ua, "chrome") || strings.Contains(ua, "safari")
}

// SecureHeaders adds security headers to the response.
func SecureHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
}

// SecurityMiddleware wraps a handler with security headers and CSRF.
func SecurityMiddleware(secret string, allowedOrigins []string) func(http.Handler) http.Handler {
	csrf := NewCSRFProtect(secret, 1*time.Hour)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			SecureHeaders(w)

			if !ValidateOrigin(r, allowedOrigins) {
				http.Error(w, `{"error":"forbidden origin"}`, http.StatusForbidden)
				return
			}

			csrf.Middleware(next).ServeHTTP(w, r)
		})
	}
}
