package admin

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Config defines admin API settings.
type Config struct {
	Addr       string        `yaml:"addr"`
	Token      string        `yaml:"token"`
	RateLimit  int           `yaml:"rate_limit"` // requests per minute
	CSRFSecret string        `yaml:"csrf_secret"`
	Timeout    time.Duration `yaml:"timeout"`
}

// DefaultAdminConfig returns sensible defaults.
func DefaultAdminConfig() *Config {
	return &Config{
		Addr:      "127.0.0.1:9090",
		RateLimit: 60,
		Timeout:   30 * time.Second,
	}
}

// Server is the admin API server.
type Server struct {
	cfg     Config
	logger  *slog.Logger
	status  *StatusProvider
	audit   *AuditLog
	limiter *RateLimiter
	mux     *http.ServeMux
}

// NewServer creates an admin API server.
func NewServer(cfg Config, logger *slog.Logger, status *StatusProvider) *Server {
	s := &Server{
		cfg:     cfg,
		logger:  logger,
		status:  status,
		audit:   NewAuditLog(),
		limiter: NewRateLimiter(cfg.RateLimit),
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/config/validate", s.handleConfigValidate)
	s.mux.HandleFunc("/api/runtime/list", s.handleRuntimeList)
	s.mux.HandleFunc("/api/metrics", s.handleMetrics)
	s.mux.HandleFunc("/api/audit", s.handleAuditLog)
	s.mux.HandleFunc("/api/health", s.handleHealth)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Rate limit.
	if !s.limiter.Allow(r.RemoteAddr) {
		http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
		return
	}

	// Auth.
	if !s.Authenticate(r) {
		s.audit.Log("auth_failed", r.RemoteAddr, r.URL.Path)
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	s.audit.Log("access", r.RemoteAddr, r.URL.Path)
	s.mux.ServeHTTP(w, r)
}

// Authenticate reports whether a request carries a valid bearer token.
//
// A missing token configuration fails closed. It used to return true, meaning a
// server constructed without a token was wide open — the failure mode where a
// misconfiguration produces no error and no warning, only silent exposure.
// §5.6 requires unsafe options to be explicit and to produce warnings; an empty
// token is not an opt-in to anything.
func (s *Server) Authenticate(r *http.Request) bool {
	if s.cfg.Token == "" {
		return false
	}

	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}

	token := strings.TrimPrefix(auth, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.Token)) == 1
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status := map[string]interface{}{
		"status":     "ok",
		"uptime":     time.Since(s.status.StartTime).String(),
		"version":    s.status.Version,
		"pid":        s.status.PID,
		"goroutines": s.status.Goroutines(),
	}

	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleConfigValidate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// For now, just acknowledge validation.
	json.NewEncoder(w).Encode(map[string]string{
		"status": "valid",
	})
}

func (s *Server) handleRuntimeList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"runtimes": s.status.Runtimes,
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	metrics := map[string]interface{}{
		"requests_total":  s.status.RequestsTotal,
		"errors_total":    s.status.ErrorsTotal,
		"active_requests": s.status.ActiveRequests,
		"avg_response_ms": s.status.AvgResponseMs,
	}

	json.NewEncoder(w).Encode(metrics)
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	entries := s.audit.Recent(100)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": entries,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// StatusProvider exposes gateway status for the admin API.
type StatusProvider struct {
	StartTime      time.Time
	Version        string
	PID            int
	Runtimes       []string
	RequestsTotal  int64
	ErrorsTotal    int64
	ActiveRequests int64
	AvgResponseMs  float64
}

// NewStatusProvider creates a status provider.
func NewStatusProvider(version string) *StatusProvider {
	return &StatusProvider{
		StartTime: time.Now(),
		Version:   version,
		PID:       os.Getpid(),
	}
}

// Goroutines returns the current goroutine count.
func (sp *StatusProvider) Goroutines() int {
	return runtime.NumGoroutine()
}

// AuditEntry is a single audit log entry.
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Remote    string    `json:"remote"`
	Path      string    `json:"path"`
}

// AuditLog stores recent admin API actions.
type AuditLog struct {
	mu      sync.RWMutex
	entries []AuditEntry
	maxSize int
}

// NewAuditLog creates an audit log.
func NewAuditLog() *AuditLog {
	return &AuditLog{maxSize: 1000}
}

// Log records an action.
func (a *AuditLog) Log(action, remote, path string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.entries = append(a.entries, AuditEntry{
		Timestamp: time.Now(),
		Action:    action,
		Remote:    remote,
		Path:      path,
	})

	if len(a.entries) > a.maxSize {
		a.entries = a.entries[len(a.entries)-a.maxSize:]
	}
}

// Recent returns the last n entries.
func (a *AuditLog) Recent(n int) []AuditEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if n > len(a.entries) {
		n = len(a.entries)
	}

	result := make([]AuditEntry, n)
	copy(result, a.entries[len(a.entries)-n:])
	return result
}

// RateLimiter implements token bucket rate limiting per key.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    int // tokens per minute
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

// NewRateLimiter creates a rate limiter with the given requests per minute.
func NewRateLimiter(rate int) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
	}
}

// Allow checks if a request from the given key is allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(rl.rate), lastCheck: time.Now()}
		rl.buckets[key] = b
	}

	elapsed := time.Since(b.lastCheck).Seconds()
	b.tokens += elapsed * float64(rl.rate) / 60.0
	if b.tokens > float64(rl.rate) {
		b.tokens = float64(rl.rate)
	}
	b.lastCheck = time.Now()

	if b.tokens < 1 {
		return false
	}

	b.tokens--
	return true
}

// Cleanup removes old buckets.
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-5 * time.Minute)
	for k, b := range rl.buckets {
		if b.lastCheck.Before(cutoff) {
			delete(rl.buckets, k)
		}
	}
}
