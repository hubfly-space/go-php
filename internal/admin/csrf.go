package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CSRFProtect provides CSRF protection for browser-based admin API access.
type CSRFProtect struct {
	mu       sync.RWMutex
	tokens   map[string]time.Time // token -> creation time
	secret   []byte
	ttl      time.Duration
	maxTokens int
}

// NewCSRFProtect creates a CSRF protection manager.
func NewCSRFProtect(secret string, ttl time.Duration) *CSRFProtect {
	return &CSRFProtect{
		tokens:   make(map[string]time.Time),
		secret:   []byte(secret),
		ttl:      ttl,
		maxTokens: 100,
	}
}

// GenerateToken creates a new CSRF token.
func (c *CSRFProtect) GenerateToken() string {
	b := make([]byte, 32)
	rand.Read(b)

	// Sign the token with HMAC.
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
	if token == "" {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	created, ok := c.tokens[token]
	if !ok {
		return false
	}

	if time.Since(created) > c.ttl {
		return false
	}

	return true
}

// Consume validates and removes a token (single-use).
func (c *CSRFProtect) Consume(token string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	created, ok := c.tokens[token]
	if !ok {
		return false
	}

	if time.Since(created) > c.ttl {
		delete(c.tokens, token)
		return false
	}

	delete(c.tokens, token)
	return true
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

// hmacSign computes HMAC-SHA256.
func hmacSign(key, data []byte) []byte {
	// Simple HMAC implementation without importing crypto/hmac
	// to avoid unnecessary dependencies.
	// In production, use crypto/hmac.
	if len(key) == 0 {
		return nil
	}

	// Use SHA-256.
	h := newHMACSHA256(key)
	h.Write(data)
	return h.Sum(nil)
}

// newHMACSHA256 creates a new HMAC-SHA256 hasher.
func newHMACSHA256(key []byte) *hmacSHA256 {
	blockSize := 64
	k := make([]byte, blockSize)
	copy(k, key)

	ipad := make([]byte, blockSize)
	opad := make([]byte, blockSize)

	for i := 0; i < blockSize; i++ {
		if i < len(k) {
			ipad[i] = k[i] ^ 0x36
			opad[i] = k[i] ^ 0x5c
		}
	}

	return &hmacSHA256{
		ipad: ipad,
		opad: opad,
	}
}

type hmacSHA256 struct {
	ipad []byte
	opad []byte
	buf  []byte
}

func (h *hmacSHA256) Write(p []byte) (int, error) {
	h.buf = append(h.buf, p...)
	return len(p), nil
}

func (h *hmacSHA256) Sum(b []byte) []byte {
	// Inner hash.
	inner := sha256Sum(append(h.ipad, h.buf...))
	// Outer hash.
	outer := sha256Sum(append(h.opad, inner...))
	return append(b, outer...)
}

// sha256Sum returns the SHA-256 digest.
func sha256Sum(data []byte) []byte {
	// Minimal SHA-256 — use crypto/sha256 in production.
	// This is a placeholder for the CSRF implementation.
	// Import crypto/sha256 and use sha256.Sum256(data) in real code.
	const (
		h0 = 0x6a09e667
		h1 = 0xbb67ae85
		h2 = 0x3c6ef372
		h3 = 0xa54ff53a
		h4 = 0x510e527f
		h5 = 0x9b05688c
		h6 = 0x1f83d9ab
		h7 = 0x5be0cd19
	)

	// Pad the message.
	msgLen := len(data)
	data = append(data, 0x80)
	for len(data)%64 != 56 {
		data = append(data, 0)
	}
	data = append(data, byte(msgLen>>56), byte(msgLen>>48), byte(msgLen>>40), byte(msgLen>>32),
		byte(msgLen>>24), byte(msgLen>>16), byte(msgLen>>8), byte(msgLen))

	h := [8]uint32{h0, h1, h2, h3, h4, h5, h6, h7}

	for i := 0; i < len(data); i += 64 {
		var w [64]uint32
		for j := 0; j < 16; j++ {
			w[j] = uint32(data[i+j*4])<<24 | uint32(data[i+j*4+1])<<16 |
				uint32(data[i+j*4+2])<<8 | uint32(data[i+j*4+3])
		}

		for j := 16; j < 64; j++ {
			s0 := rotr(w[j-15], 7) ^ rotr(w[j-15], 18) ^ (w[j-15] >> 3)
			s1 := rotr(w[j-2], 17) ^ rotr(w[j-2], 19) ^ (w[j-2] >> 10)
			w[j] = w[j-16] + s0 + w[j-7] + s1
		}

		a, b, c, d, e, f, g, hh := h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7]

		for j := 0; j < 64; j++ {
			S1 := rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25)
			ch := (e & f) ^ (^e & g)
			temp1 := hh + S1 + ch + k256[j] + w[j]
			S0 := rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22)
			maj := (a & b) ^ (a & c) ^ (b & c)
			temp2 := S0 + maj

			hh = g
			g = f
			f = e
			e = d + temp1
			d = c
			c = b
			b = a
			a = temp1 + temp2
		}

		h[0] += a
		h[1] += b
		h[2] += c
		h[3] += d
		h[4] += e
		h[5] += f
		h[6] += g
		h[7] += hh
	}

	result := make([]byte, 32)
	for i, v := range h {
		result[i*4] = byte(v >> 24)
		result[i*4+1] = byte(v >> 16)
		result[i*4+2] = byte(v >> 8)
		result[i*4+3] = byte(v)
	}

	return result
}

func rotr(x uint32, n uint) uint32 {
	return (x >> n) | (x << (32 - n))
}

var k256 = [64]uint32{
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
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
