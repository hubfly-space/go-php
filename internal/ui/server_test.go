package ui

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	status := NewStatusProvider("test", ":8080", "/var/www", "Laravel")
	status.Runtimes = []string{"8.3", "8.2"}
	sites := []SiteConfig{
		{ID: "my-site", Name: "My Site", Domain: "example.com", Root: "/var/www/html", Status: "active", PHPVersion: "8.3"},
	}
	status.Sites.Store(&sites)
	return NewServer(DefaultConfig(), logger, status)
}

func TestHandleStatus(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	s.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
	if resp["version"] != "test" {
		t.Errorf("expected version test, got %v", resp["version"])
	}
}

func TestHandleSites(t *testing.T) {
	s := newTestServer(t)

	// GET sites
	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	w := httptest.NewRecorder()
	s.handleSites(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	sites := resp["sites"].([]any)
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(sites))
	}

	// POST new site
	body := `{"name":"New Site","domain":"new.com","root":"/var/www/new"}`
	req = httptest.NewRequest(http.MethodPost, "/api/sites", strings.NewReader(body))
	w = httptest.NewRecorder()
	s.handleSites(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var createResp map[string]any
	json.NewDecoder(w.Body).Decode(&createResp)
	site := createResp["site"].(map[string]any)
	if site["name"] != "New Site" {
		t.Errorf("expected name New Site, got %v", site["name"])
	}
}

func TestHandleSiteByID(t *testing.T) {
	s := newTestServer(t)

	// GET existing
	req := httptest.NewRequest(http.MethodGet, "/api/sites/my-site", nil)
	w := httptest.NewRecorder()
	s.handleSiteByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// GET non-existing
	req = httptest.NewRequest(http.MethodGet, "/api/sites/does-not-exist", nil)
	w = httptest.NewRecorder()
	s.handleSiteByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	// DELETE
	req = httptest.NewRequest(http.MethodDelete, "/api/sites/my-site", nil)
	w = httptest.NewRecorder()
	s.handleSiteByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleConfig(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	s.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var cfg GatewayConfig
	json.NewDecoder(w.Body).Decode(&cfg)
	if cfg.Schema != "gateway/v1" {
		t.Errorf("expected schema gateway/v1, got %v", cfg.Schema)
	}
}

func TestHandleRuntimes(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/runtimes", nil)
	w := httptest.NewRecorder()
	s.handleRuntimes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	runtimes := resp["runtimes"].([]any)
	if len(runtimes) != 2 {
		t.Fatalf("expected 2 runtimes, got %d", len(runtimes))
	}
}

func TestHandleHealth(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleSystem(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/system", nil)
	w := httptest.NewRecorder()
	s.handleSystem(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var info SystemInfo
	json.NewDecoder(w.Body).Decode(&info)
	if info.PID != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), info.PID)
	}
}

func TestHandleLogs(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	w := httptest.NewRecorder()
	s.handleLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	s := newTestServer(t)

	// POST on GET-only endpoint
	req := httptest.NewRequest(http.MethodPost, "/api/status", nil)
	w := httptest.NewRecorder()
	s.handleStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestLogBuffer(t *testing.T) {
	lb := NewLogBuffer(3)

	lb.Add(LogEntry{Level: "info", Message: "a"})
	lb.Add(LogEntry{Level: "warn", Message: "b"})
	lb.Add(LogEntry{Level: "error", Message: "c"})
	lb.Add(LogEntry{Level: "info", Message: "d"})

	if lb.Size() != 3 {
		t.Fatalf("expected size 3, got %d", lb.Size())
	}

	recent := lb.Recent(2)
	if len(recent) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(recent))
	}
	if recent[0].Message != "c" {
		t.Errorf("expected first entry 'c', got %v", recent[0].Message)
	}
	if recent[1].Message != "d" {
		t.Errorf("expected second entry 'd', got %v", recent[1].Message)
	}
}

func TestGenerateID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Site", "my-site"},
		{"Hello World!", "hello-world"},
		{"test123", "test123"},
		{"", "site"},
		{"UPPER CASE", "upper-case"},
	}
	for _, tt := range tests {
		got := generateID(tt.input)
		if got != tt.want {
			t.Errorf("generateID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
