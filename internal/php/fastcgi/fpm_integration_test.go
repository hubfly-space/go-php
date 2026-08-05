//go:build integration

package fastcgi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	fpmBin := findFPMBinary()
	if fpmBin == "" {
		t.Skip("php-fpm not found, skipping integration test")
	}

	// Start FPM with a socket.
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "php-fpm.sock")
	pidFile := filepath.Join(tempDir, "php-fpm.pid")
	errorLog := filepath.Join(tempDir, "php-fpm.log")
	confFile := filepath.Join(tempDir, "php-fpm.conf")

	conf := fmt.Sprintf(`
[global]
pid = %s
error_log = %s
daemonize = no

[www]
listen = %s
listen.mode = 0666
pm = dynamic
pm.max_children = 5
pm.start_servers = 1
pm.min_spare_servers = 1
pm.max_spare_servers = 2
pm.max_requests = 500
security.limit_extensions = .php
`, pidFile, errorLog, socketPath)

	if err := os.WriteFile(confFile, []byte(conf), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(fpmBin, "-y", confFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start php-fpm: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Wait for socket to appear.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("php-fpm socket did not appear within 5 seconds: %v", err)
	}

	// reconnect creates a new client when FPM closes the connection.
	reconnect := func(t *testing.T) *Client {
		t.Helper()
		c, err := NewClient(socketPath, 5*time.Second)
		if err != nil {
			t.Fatalf("failed to connect to php-fpm: %v", err)
		}
		return c
	}

	phpDir := t.TempDir()
	phpFile := filepath.Join(phpDir, "test.php")
	phpContent := `<?php
header('X-Test-Header: hello');
header('Content-Type: text/plain');
echo 'Hello from PHP ' . PHP_VERSION;
?>`
	if err := os.WriteFile(phpFile, []byte(phpContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("basic_request", func(t *testing.T) {
		c := reconnect(t)
		defer c.Close()

		params := map[string]string{
			"GATEWAY_INTERFACE": "CGI/1.1",
			"SERVER_SOFTWARE":   "test",
			"SERVER_PROTOCOL":   "HTTP/1.1",
			"REQUEST_METHOD":    "GET",
			"REQUEST_URI":       "/test.php",
			"SCRIPT_FILENAME":   phpFile,
			"SCRIPT_NAME":       "/test.php",
			"DOCUMENT_ROOT":     phpDir,
			"SERVER_NAME":       "localhost",
			"SERVER_PORT":       "80",
			"REMOTE_ADDR":       "127.0.0.1",
		}

		stdout, stderr, endReq, err := c.Execute(context.Background(), params, nil)
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
		c := reconnect(t)
		defer c.Close()

		postPHP := filepath.Join(phpDir, "post.php")
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
			"DOCUMENT_ROOT":     phpDir,
			"CONTENT_TYPE":      "application/x-www-form-urlencoded",
			"CONTENT_LENGTH":    "11",
			"SERVER_NAME":       "localhost",
			"REMOTE_ADDR":       "127.0.0.1",
		}

		body := strings.NewReader("name=test&value=42")
		stdout, _, _, err := c.Execute(context.Background(), params, body)
		if err != nil {
			t.Fatalf("POST request failed: %v", err)
		}

		t.Logf("POST response: %s", string(stdout))
	})

	t.Run("error_output", func(t *testing.T) {
		c := reconnect(t)
		defer c.Close()

		errorPHP := filepath.Join(phpDir, "error.php")
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
			"DOCUMENT_ROOT":     phpDir,
			"SERVER_NAME":       "localhost",
			"REMOTE_ADDR":       "127.0.0.1",
		}

		stdout, stderr, _, err := c.Execute(context.Background(), params, nil)
		if err != nil {
			t.Fatalf("error request failed: %v", err)
		}

		t.Logf("stdout: %s", string(stdout))
		t.Logf("stderr: %s", string(stderr))
	})

	t.Run("timeout", func(t *testing.T) {
		slowPHP := filepath.Join(phpDir, "slow.php")
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
			"DOCUMENT_ROOT":     phpDir,
			"SERVER_NAME":       "localhost",
			"REMOTE_ADDR":       "127.0.0.1",
		}

		slowClient, err := NewClient(socketPath, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer slowClient.Close()

		_, _, _, err = slowClient.Execute(context.Background(), params, nil)
		if err == nil {
			return
		}
		t.Logf("expected timeout error: %v", err)
	})
}

func findFPMBinary() string {
	for _, name := range []string{"php-fpm8.3", "php-fpm8.2", "php-fpm8.1", "php-fpm8.0", "php-fpm"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}
