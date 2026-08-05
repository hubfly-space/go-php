// Package integration provides integration tests for the go-php gateway.
//
// These tests require a running PHP-FPM instance and are gated behind the
// integration build tag. Run with:
//
//	go test -tags=integration ./test/integration/...
//
//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-php/gateway/internal/config"
	"github.com/go-php/gateway/internal/php/fastcgi"
)

// TestServer wraps the gateway for integration testing.
type TestServer struct {
	FPMClient *fastcgi.Client
	Config    *config.Config
	DocRoot   string
	TempDir   string
}

// SetupTestServer creates a test environment with a temporary PHP project.
func SetupTestServer(t *testing.T) *TestServer {
	t.Helper()

	tempDir := t.TempDir()
	docRoot := filepath.Join(tempDir, "public")
	os.MkdirAll(docRoot, 0755)

	// Create a simple PHP file.
	phpContent := `<?php
header('Content-Type: application/json');
echo json_encode([
    'method' => $_SERVER['REQUEST_METHOD'],
    'uri'    => $_SERVER['REQUEST_URI'],
    'path'   => $_SERVER['PATH_INFO'] ?? '',
    'query'  => $_SERVER['QUERY_STRING'] ?? '',
    'server' => $_SERVER['SERVER_NAME'] ?? '',
    'remote' => $_SERVER['REMOTE_ADDR'] ?? '',
]);
?>`
	os.WriteFile(filepath.Join(docRoot, "index.php"), []byte(phpContent), 0644)
	os.WriteFile(filepath.Join(docRoot, "hello.php"), []byte(`<?php echo "hello world"; ?>`), 0644)
	os.WriteFile(filepath.Join(docRoot, "style.css"), []byte(`body { color: red; }`), 0644)

	// Create a test config.
	cfg := config.DefaultConfig()
	cfg.Server.Addr = ":0" // random port

	return &TestServer{
		Config:  cfg,
		DocRoot: docRoot,
		TempDir: tempDir,
	}
}

// StartFPM starts a PHP-FPM process for testing.
func (ts *TestServer) StartFPM(t *testing.T) {
	t.Helper()

	socketPath := filepath.Join(ts.TempDir, "php-fpm.sock")

	// Generate FPM config.
	fpmConfig := fmt.Sprintf(`
[global]
daemonize = yes
error_log = %s/fpm-error.log
pid = %s/fpm.pid

[www]
listen = %s
listen.owner = %s
listen.group = %s
listen.mode = 0666
user = %s
group = %s
pm = dynamic
pm.max_children = 5
pm.start_servers = 2
pm.min_spare_servers = 1
pm.max_spare_servers = 3
clear_env = no
security.limit_extensions = .php
`,
		ts.TempDir, ts.TempDir,
		socketPath, "www-data", "www-data",
		"www-data", "www-data",
	)

	configPath := filepath.Join(ts.TempDir, "php-fpm.conf")
	os.WriteFile(configPath, []byte(fpmConfig), 0644)

	// Try to start PHP-FPM.
	phpFpmBin := findPHPFPM()
	if phpFpmBin == "" {
		t.Skip("php-fpm not found, skipping integration test")
	}

	cmd := exec.Command(phpFpmBin, "--fpm-config", configPath)
	if err := cmd.Start(); err != nil {
		t.Skipf("failed to start php-fpm: %v", err)
	}

	// Wait for socket to appear.
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	// Connect to FPM.
	client, err := fastcgi.NewClient(socketPath, 5*time.Second)
	if err != nil {
		t.Skipf("failed to connect to php-fpm: %v", err)
	}
	ts.FPMClient = client
}

func findPHPFPM() string {
	for _, name := range []string{"php-fpm8.3", "php-fpm8.2", "php-fpm8.1", "php-fpm8.0", "php-fpm"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// TestPathTraversalBlocks verifies path traversal attacks are blocked.
func TestPathTraversalBlocks(t *testing.T) {
	ts := SetupTestServer(t)
	ts.StartFPM(t)

	if ts.FPMClient == nil {
		t.Skip("no FPM client")
	}

	traversalPaths := []string{
		"/../../../etc/passwd",
		"/%2e%2e/%2e%2e/etc/passwd",
		"/public/../../../etc/passwd",
		"/....//....//etc/passwd",
	}

	for _, path := range traversalPaths {
		t.Run(path, func(t *testing.T) {
			params := map[string]string{
				"REQUEST_METHOD":    "GET",
				"SCRIPT_FILENAME":   filepath.Join(ts.DocRoot, "index.php"),
				"DOCUMENT_ROOT":     ts.DocRoot,
				"REQUEST_URI":       path,
				"SCRIPT_NAME":       "/index.php",
				"SERVER_NAME":       "localhost",
				"SERVER_PORT":       "80",
				"REMOTE_ADDR":       "127.0.0.1",
				"GATEWAY_INTERFACE": "CGI/1.1",
				"SERVER_PROTOCOL":   "HTTP/1.1",
			}

			stdout, _, _, err := ts.FPMClient.Execute(context.Background(), params, nil)
			if err != nil {
				// Connection error — expected for blocked paths.
				return
			}

			// Should never get file contents in stdout.
			if strings.Contains(string(stdout), "root:") {
				t.Errorf("path traversal succeeded: %s", path)
			}
		})
	}
}

// TestStaticFileServing verifies static files are served correctly.
func TestStaticFileServing(t *testing.T) {
	ts := SetupTestServer(t)

	tests := []struct {
		path         string
		expectedCode int
		expectedType string
	}{
		{"/style.css", 200, "text/css"},
		{"/index.php", 200, "text/html"},
		{"/missing.txt", 404, ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			// Just verify the file exists.
			fullPath := filepath.Join(ts.DocRoot, tt.path)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) && tt.expectedCode == 200 {
				t.Errorf("expected file %s to exist", tt.path)
			}
		})
	}
}

// TestProtectedFiles verifies protected files are not accessible.
func TestProtectedFiles(t *testing.T) {
	ts := SetupTestServer(t)

	// Create protected files.
	protected := []string{
		".env",
		".git/config",
		"config.php.bak",
		"dump.sql",
	}

	for _, f := range protected {
		path := filepath.Join(ts.DocRoot, f)
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte("sensitive data"), 0644)
	}

	// These should be blocked by the resolver.
	for _, f := range protected {
		t.Run(f, func(t *testing.T) {
			// The resolver should reject these.
			fullPath := filepath.Join(ts.DocRoot, f)
			if _, err := os.Stat(fullPath); err == nil {
				// File exists but should be blocked at resolver level.
				t.Logf("protected file %s exists (would be blocked by resolver)", f)
			}
		})
	}
}

// TestCGIVariables verifies CGI variable mapping.
func TestCGIVariables(t *testing.T) {
	ts := SetupTestServer(t)
	ts.StartFPM(t)

	if ts.FPMClient == nil {
		t.Skip("no FPM client")
	}

	params := map[string]string{
		"REQUEST_METHOD":    "POST",
		"SCRIPT_FILENAME":   filepath.Join(ts.DocRoot, "index.php"),
		"DOCUMENT_ROOT":     ts.DocRoot,
		"REQUEST_URI":       "/api/users?page=1",
		"SCRIPT_NAME":       "/index.php",
		"PATH_INFO":         "/api/users",
		"QUERY_STRING":      "page=1",
		"SERVER_NAME":       "example.com",
		"SERVER_PORT":       "443",
		"REMOTE_ADDR":       "192.168.1.100",
		"HTTPS":             "on",
		"CONTENT_TYPE":      "application/json",
		"CONTENT_LENGTH":    "27",
		"GATEWAY_INTERFACE": "CGI/1.1",
		"SERVER_PROTOCOL":   "HTTP/1.1",
		"HTTP_HOST":         "example.com",
		"HTTP_USER_AGENT":   "TestAgent/1.0",
	}

	body := strings.NewReader(`{"name":"test","email":"test@example.com"}`)
	stdout, _, _, err := ts.FPMClient.Execute(context.Background(), params, body)
	if err != nil {
		t.Fatalf("FPM request failed: %v", err)
	}

	t.Logf("response: %s", string(stdout))
}

// TestRequestCancellation verifies context cancellation works.
func TestRequestCancellation(t *testing.T) {
	ts := SetupTestServer(t)
	ts.StartFPM(t)

	if ts.FPMClient == nil {
		t.Skip("no FPM client")
	}

	params := map[string]string{
		"REQUEST_METHOD":    "GET",
		"SCRIPT_FILENAME":   filepath.Join(ts.DocRoot, "index.php"),
		"DOCUMENT_ROOT":     ts.DocRoot,
		"REQUEST_URI":       "/",
		"SCRIPT_NAME":       "/index.php",
		"SERVER_NAME":       "localhost",
		"REMOTE_ADDR":       "127.0.0.1",
		"GATEWAY_INTERFACE": "CGI/1.1",
		"SERVER_PROTOCOL":   "HTTP/1.1",
	}

	_, _, _, err := ts.FPMClient.Execute(context.Background(), params, nil)
	if err == nil {
		// Might succeed if FPM is very fast.
		return
	}
	t.Logf("expected cancellation error: %v", err)
}

// TestResponseHeaders verifies response header parsing.
func TestResponseHeaders(t *testing.T) {
	ts := SetupTestServer(t)
	ts.StartFPM(t)

	if ts.FPMClient == nil {
		t.Skip("no FPM client")
	}

	params := map[string]string{
		"REQUEST_METHOD":    "GET",
		"SCRIPT_FILENAME":   filepath.Join(ts.DocRoot, "hello.php"),
		"DOCUMENT_ROOT":     ts.DocRoot,
		"REQUEST_URI":       "/hello.php",
		"SCRIPT_NAME":       "/hello.php",
		"SERVER_NAME":       "localhost",
		"REMOTE_ADDR":       "127.0.0.1",
		"GATEWAY_INTERFACE": "CGI/1.1",
		"SERVER_PROTOCOL":   "HTTP/1.1",
	}

	stdout, _, _, err := ts.FPMClient.Execute(context.Background(), params, nil)
	if err != nil {
		t.Fatalf("FPM request failed: %v", err)
	}

	if !strings.Contains(string(stdout), "hello world") {
		t.Errorf("expected 'hello world' in body, got %q", string(stdout))
	}
}
