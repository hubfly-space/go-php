package admin

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GuardConfig configures a Guard.
type GuardConfig struct {
	// Token is the bearer token required on every request. Empty means the
	// guard denies everything — see Guard.Wrap.
	Token string

	// AllowedOrigins are the origins permitted to make browser requests. An
	// empty list means no cross-origin browser request is accepted.
	AllowedOrigins []string

	// RateLimit is the per-client request budget per minute.
	RateLimit int

	// PublicPaths are exact paths served without authentication, for
	// unauthenticated liveness probes. Keep this list tiny.
	PublicPaths []string
}

// DefaultGuardConfig returns a guard configuration with safe defaults.
func DefaultGuardConfig() GuardConfig {
	return GuardConfig{
		RateLimit:   600,
		PublicPaths: []string{"/api/health"},
	}
}

// Guard is authentication, origin validation, rate limiting, audit logging, and
// security headers, packaged for wrapping a management mux.
//
// It exists because internal/admin had all of this and was imported by nothing,
// while the management API that actually runs had none of it.
type Guard struct {
	cfg     GuardConfig
	logger  *slog.Logger
	audit   *AuditLog
	limiter *RateLimiter

	publicPaths map[string]bool

	mu       sync.Mutex
	denials  int
	failures map[string]int
}

// NewGuard creates a Guard.
func NewGuard(cfg GuardConfig, logger *slog.Logger) *Guard {
	if cfg.RateLimit <= 0 {
		cfg.RateLimit = DefaultGuardConfig().RateLimit
	}

	public := make(map[string]bool, len(cfg.PublicPaths))
	for _, p := range cfg.PublicPaths {
		public[p] = true
	}

	return &Guard{
		cfg:         cfg,
		logger:      logger,
		audit:       NewAuditLog(),
		limiter:     NewRateLimiter(cfg.RateLimit),
		publicPaths: public,
		failures:    make(map[string]int),
	}
}

// Audit exposes the audit log for status reporting.
func (g *Guard) Audit() *AuditLog { return g.audit }

// Denials returns how many requests the guard has rejected.
func (g *Guard) Denials() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.denials
}

func (g *Guard) deny(w http.ResponseWriter, r *http.Request, status int, action, msg string) {
	g.mu.Lock()
	g.denials++
	g.mu.Unlock()

	g.audit.Log(action, r.RemoteAddr, r.URL.Path)
	if g.logger != nil {
		g.logger.Warn("management API request denied",
			"action", action, "remote", clientHost(r), "path", r.URL.Path)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`+"\n", msg)
}

// authenticate reports whether the request carries the configured bearer token.
// It is deliberately fail-closed when no token is configured.
func (g *Guard) authenticate(r *http.Request) bool {
	if g.cfg.Token == "" {
		return false
	}

	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		// Accept a query parameter only for the WebSocket upgrade, because the
		// browser WebSocket API cannot set request headers.
		if isWebSocketUpgrade(r) {
			if t := r.URL.Query().Get("token"); t != "" {
				return subtle.ConstantTimeCompare([]byte(t), []byte(g.cfg.Token)) == 1
			}
		}
		return false
	}

	token := strings.TrimPrefix(auth, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(token), []byte(g.cfg.Token)) == 1
}

// isWebSocketUpgrade reports whether the request is a WebSocket handshake.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// CheckOrigin reports whether a WebSocket handshake's Origin is acceptable.
//
// The upgrader previously accepted every origin, so any page open in the
// operator's browser could attach to the management socket. Same-origin
// requests and non-browser clients (no Origin header) are allowed; anything
// else must be listed explicitly.
func (g *Guard) CheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if sameOrigin(origin, r.Host) {
		return true
	}
	for _, allowed := range g.cfg.AllowedOrigins {
		if subtle.ConstantTimeCompare([]byte(origin), []byte(allowed)) == 1 {
			return true
		}
	}
	return false
}

// sameOrigin reports whether an Origin header refers to the request's own host.
func sameOrigin(origin, host string) bool {
	trimmed := origin
	for _, prefix := range []string{"http://", "https://"} {
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimPrefix(trimmed, prefix)
			break
		}
	}
	return trimmed == host
}

// clientHost returns the peer address without its port, for logging.
func clientHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Wrap returns next protected by the guard.
func (g *Guard) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SecureHeaders(w)

		if !g.limiter.Allow(clientHost(r)) {
			w.Header().Set("Retry-After", "60")
			g.deny(w, r, http.StatusTooManyRequests, "rate_limited", "rate limited")
			return
		}

		// Origin is checked before auth so a cross-origin page cannot use the
		// response status to probe whether a guessed token was correct.
		if !g.CheckOrigin(r) {
			g.deny(w, r, http.StatusForbidden, "origin_rejected", "forbidden origin")
			return
		}

		if g.publicPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		if !g.authenticate(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="gateway management API"`)
			g.deny(w, r, http.StatusUnauthorized, "auth_failed", "unauthorized")
			return
		}

		// Record mutations only; logging every poll would drown the audit log.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			g.audit.Log(r.Method+" "+r.URL.Path, r.RemoteAddr, r.URL.Path)
		}

		next.ServeHTTP(w, r)
	})
}

// GenerateToken returns a new random management token.
func GenerateToken() string { return GenerateSecret(32) }

// TokenTTL is how long a generated CSRF token remains valid.
const TokenTTL = time.Hour
