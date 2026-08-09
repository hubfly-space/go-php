package main

import (
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-php/gateway/internal/config"
)

func TestParseFlagsPermuted(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	phpFPM := fs.String("php-fpm", "", "php fpm path")

	args := []string{".", "--php-fpm", "/usr/sbin/php-fpm8.3"}
	positional, err := parseFlagsPermuted(fs, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(positional) != 1 || positional[0] != "." {
		t.Errorf("expected positional [.], got %v", positional)
	}

	if *phpFPM != "/usr/sbin/php-fpm8.3" {
		t.Errorf("expected php-fpm flag /usr/sbin/php-fpm8.3, got %q", *phpFPM)
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"localhost:30200", true},
		{"[::1]:8080", true},
		{"0.0.0.0:8080", false},
		{"192.168.1.1:8080", false},
		{":8080", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		got := isLoopbackAddr(tt.addr)
		if got != tt.want {
			t.Errorf("isLoopbackAddr(%q) = %v; want %v", tt.addr, got, tt.want)
		}
	}
}

func TestDetectMIME(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"index.html", "text/html; charset=utf-8"},
		{"style.css", "text/css; charset=utf-8"},
		{"app.js", "application/javascript; charset=utf-8"},
		{"data.json", "application/json; charset=utf-8"},
		{"image.png", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"unknown.xyz", "application/octet-stream"},
	}

	for _, tt := range tests {
		got := detectMIME(tt.path)
		if got != tt.want {
			t.Errorf("detectMIME(%q) = %q; want %q", tt.path, got, tt.want)
		}
	}
}

func TestDetectFramework(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty dir: no framework.
	fw, root := detectFramework(tmpDir)
	if fw != "" || root != "" {
		t.Errorf("expected empty detection for empty dir, got %q, %q", fw, root)
	}

	// Laravel detection via artisan and public dir.
	if err := os.WriteFile(filepath.Join(tmpDir, "artisan"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "public"), 0755); err != nil {
		t.Fatal(err)
	}

	fw, root = detectFramework(tmpDir)
	if fw != "Laravel" {
		t.Errorf("expected Laravel framework, got %q", fw)
	}
	if root != filepath.Join(tmpDir, "public") {
		t.Errorf("expected pubRoot %q, got %q", filepath.Join(tmpDir, "public"), root)
	}
}

func TestDevErrorXSSEscaping(t *testing.T) {
	h := &gatewayHandler{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/<script>alert('xss')</script>", nil)

	start := time.Now()
	h.devError(w, r, 400, "Bad Request", "<img src=x onerror=alert(1)>", "req_<script>", start)

	body := w.Body.String()

	if strings.Contains(body, "<script>alert('xss')</script>") {
		t.Errorf("reflected XSS in Path output! Body: %s", body)
	}
	if strings.Contains(body, "<img src=x onerror=alert(1)>") {
		t.Errorf("reflected XSS in Detail output! Body: %s", body)
	}

	if !strings.Contains(body, "&lt;script&gt;") && !strings.Contains(body, "%3Cscript%3E") {
		t.Errorf("expected escaped HTML output in response body")
	}
}

func TestBuildRouter(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Routes = []config.RouteConfig{
		{
			PathPrefix: "/api/",
			Target:     "/index.php",
			Methods:    []string{"GET", "POST"},
		},
	}

	engine, err := buildRouter(cfg)
	if err != nil {
		t.Fatalf("buildRouter failed: %v", err)
	}

	req := httptest.NewRequest("GET", "http://example.com/api/users", nil)
	matched := engine.Match(req)
	if matched == nil {
		t.Fatalf("expected route match for /api/users")
	}
	if matched.Target != "/index.php" {
		t.Errorf("expected target /index.php, got %q", matched.Target)
	}
}

func TestProxyRouting(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Proxied", "true")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Proxied backend response"))
	}))
	defer backend.Close()

	cfg := config.DefaultConfig()
	cfg.Routes = []config.RouteConfig{
		{
			PathPrefix: "/proxy/",
			Target:     backend.URL,
		},
	}

	routerEngine, err := buildRouter(cfg)
	if err != nil {
		t.Fatalf("buildRouter failed: %v", err)
	}

	proxies, err := buildProxies(cfg)
	if err != nil {
		t.Fatalf("buildProxies failed: %v", err)
	}

	handler := &gatewayHandler{}
	handler.state.Store(&serveState{
		cfg:     cfg,
		router:  routerEngine,
		proxies: proxies,
	})

	req := httptest.NewRequest("GET", "/proxy/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Backend-Proxied") != "true" {
		t.Errorf("expected X-Backend-Proxied header from backend")
	}
	if rec.Body.String() != "Proxied backend response" {
		t.Errorf("expected body 'Proxied backend response', got %q", rec.Body.String())
	}
}
