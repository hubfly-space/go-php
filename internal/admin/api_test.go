package admin

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerHealth(t *testing.T) {
	status := NewStatusProvider("test")
	cfg := *DefaultAdminConfig()
	cfg.Token = "test-token"

	srv := NewServer(cfg, nil, status)

	req := httptest.NewRequest("GET", "/api/health", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Error("expected ok in response")
	}
}

func TestServerFailsClosedWithoutToken(t *testing.T) {
	// A server constructed with no token used to serve every endpoint to
	// everyone. A missing token is a misconfiguration, not consent.
	status := NewStatusProvider("test")
	cfg := *DefaultAdminConfig()
	cfg.Token = ""

	srv := NewServer(cfg, nil, status)

	for _, path := range []string{"/api/status", "/api/health", "/api/audit", "/api/metrics"} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Errorf("%s status = %d, want 401 when no token is configured", path, w.Code)
		}
	}
}

func TestServerAuth(t *testing.T) {
	status := NewStatusProvider("test")
	cfg := *DefaultAdminConfig()
	cfg.Token = "secret123"

	srv := NewServer(cfg, nil, status)

	// No token.
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("no token: status = %d, want 401", w.Code)
	}

	// Wrong token.
	req.Header.Set("Authorization", "Bearer wrong")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("wrong token: status = %d, want 401", w.Code)
	}

	// Correct token.
	req.Header.Set("Authorization", "Bearer secret123")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("correct token: status = %d, want 200", w.Code)
	}
}

func TestServerStatus(t *testing.T) {
	status := NewStatusProvider("test")
	cfg := *DefaultAdminConfig()
	cfg.Token = "test-token"

	srv := NewServer(cfg, nil, status)

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)

	if result["version"] != "test" {
		t.Errorf("version = %v, want test", result["version"])
	}
}

func TestAuditLog(t *testing.T) {
	log := NewAuditLog()
	log.Log("test_action", "127.0.0.1", "/test")

	entries := log.Recent(10)
	if len(entries) != 1 {
		t.Errorf("entries = %d, want 1", len(entries))
	}
	if entries[0].Action != "test_action" {
		t.Errorf("action = %q", entries[0].Action)
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(5) // 5 per minute

	for i := 0; i < 5; i++ {
		if !limiter.Allow("test") {
			t.Errorf("request %d should be allowed", i)
		}
	}

	if limiter.Allow("test") {
		t.Error("6th request should be rate limited")
	}
}

func TestServerAuditEndpoint(t *testing.T) {
	status := NewStatusProvider("test")
	cfg := *DefaultAdminConfig()
	cfg.Token = "test-token"

	srv := NewServer(cfg, nil, status)

	req := httptest.NewRequest("GET", "/api/audit", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestServerMetricsEndpoint(t *testing.T) {
	status := NewStatusProvider("test")
	cfg := *DefaultAdminConfig()
	cfg.Token = "test-token"

	srv := NewServer(cfg, nil, status)

	req := httptest.NewRequest("GET", "/api/metrics", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
