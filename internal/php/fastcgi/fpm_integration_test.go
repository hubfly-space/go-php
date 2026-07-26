//go:build integration

package fastcgi

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestFPMIntegration tests the FastCGI client against a real PHP-FPM instance.
// Run with: go test -tags=integration ./internal/php/fastcgi/...
func TestFPMIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping FPM integration test in short mode")
	}

	socketPath := findFPM()
	if socketPath == "" {
		t.Skip("php-fpm not found, skipping integration test")
	}

	client, err := NewClient(socketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to php-fpm at %s: %v", socketPath, err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	phpFile := tempDir + "/test.php"
	phpContent := `<?php
header('X-Test-Header: hello');
header('Content-Type: text/plain');
echo 'Hello from PHP ' . PHP_VERSION;
?>`
	if err := os.WriteFile(phpFile, []byte(phpContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("basic_request", func(t *testing.T) {
		params := map[string]string{
			"GATEWAY_INTERFACE": "CGI/1.1",
			"SERVER_SOFTWARE":   "test",
			"SERVER_PROTOCOL":   "HTTP/1.1",
			"REQUEST_METHOD":    "GET",
			"REQUEST_URI":       "/test.php",
			"SCRIPT_FILENAME":   phpFile,
			"SCRIPT_NAME":       "/test.php",
			"DOCUMENT_ROOT":     tempDir,
			"SERVER_NAME":       "localhost",
			"SERVER_PORT":       "80",
			"REMOTE_ADDR":       "127.0.0.1",
		}

		stdout, stderr, endReq, err := client.Execute(params, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if endReq == nil {
			t.Fatal("expected END_REQUEST, got nil")
		}

		t.Logf("app status: %d, protocol status: %d", endReq.AppStatus, endReq.ProtocolStatus)
		t.Logf("stdout: %s", string(stdout))
		t.Logf("stderr: %s", string(stderr))

		if !strings.Contains(string(stdout), "Hello from PHP") {
			t.Errorf("unexpected stdout: %s", string(stdout))
		}
	})

	t.Run("post_request", func(t *testing.T) {
		postPHP := tempDir + "/post.php"
		os.WriteFile(postPHP, []byte(`<?php
header('Content-Type: application/json');
echo json_encode($_POST);
?>`), 0644)

		params := map[string]string{
			"GATEWAY_INTERFACE": "CGI/1.1",
			"SERVER_PROTOCOL":   "HTTP/1.1",
			"REQUEST_METHOD":    "POST",
			"REQUEST_URI":       "/post.php",
			"SCRIPT_FILENAME":   postPHP,
			"SCRIPT_NAME":       "/post.php",
			"DOCUMENT_ROOT":     tempDir,
			"CONTENT_TYPE":      "application/x-www-form-urlencoded",
			"CONTENT_LENGTH":    "11",
			"SERVER_NAME":       "localhost",
			"REMOTE_ADDR":       "127.0.0.1",
		}

		body := strings.NewReader("name=test&value=42")
		stdout, _, _, err := client.Execute(params, body)
		if err != nil {
			t.Fatalf("POST request failed: %v", err)
		}

		t.Logf("POST response: %s", string(stdout))
	})

	t.Run("error_output", func(t *testing.T) {
		errorPHP := tempDir + "/error.php"
		os.WriteFile(errorPHP, []byte(`<?php
fwrite(STDERR, "error message\n");
echo 'partial output';
trigger_error("test warning", E_USER_WARNING);
?>`), 0644)

		params := map[string]string{
			"GATEWAY_INTERFACE": "CGI/1.1",
			"SERVER_PROTOCOL":   "HTTP/1.1",
			"REQUEST_METHOD":    "GET",
			"REQUEST_URI":       "/error.php",
			"SCRIPT_FILENAME":   errorPHP,
			"SCRIPT_NAME":       "/error.php",
			"DOCUMENT_ROOT":     tempDir,
			"SERVER_NAME":       "localhost",
			"REMOTE_ADDR":       "127.0.0.1",
		}

		stdout, stderr, _, err := client.Execute(params, nil)
		if err != nil {
			t.Fatalf("error request failed: %v", err)
		}

		t.Logf("stdout: %s", string(stdout))
		t.Logf("stderr: %s", string(stderr))
	})

	t.Run("timeout", func(t *testing.T) {
		slowPHP := tempDir + "/slow.php"
		os.WriteFile(slowPHP, []byte(`<?php
usleep(500000);
echo 'done';
?>`), 0644)

		params := map[string]string{
			"GATEWAY_INTERFACE": "CGI/1.1",
			"SERVER_PROTOCOL":   "HTTP/1.1",
			"REQUEST_METHOD":    "GET",
			"REQUEST_URI":       "/slow.php",
			"SCRIPT_FILENAME":   slowPHP,
			"SCRIPT_NAME":       "/slow.php",
			"DOCUMENT_ROOT":     tempDir,
			"SERVER_NAME":       "localhost",
			"REMOTE_ADDR":       "127.0.0.1",
		}

		slowClient, err := NewClient(socketPath, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer slowClient.Close()

		_, _, _, err = slowClient.Execute(params, nil)
		if err == nil {
			return
		}
		t.Logf("expected timeout error: %v", err)
	})
}

func findFPM() string {
	for _, name := range []string{"php-fpm8.3", "php-fpm8.2", "php-fpm8.1", "php-fpm8.0", "php-fpm"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}
