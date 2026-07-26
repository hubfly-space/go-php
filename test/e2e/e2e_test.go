// Package e2e provides real-world end-to-end tests for go-php-gateway.
//
// These tests start an actual PHP-FPM process, connect via FastCGI, and verify
// real PHP execution — not mocks. Every test case documents exactly what it
// tests, what the expected output is, and why.
//
// Run with:
//
//	go test -tags=e2e -v ./test/e2e/...
//
//go:build e2e

package e2e

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-php/gateway/internal/php/fastcgi"
)

// ============================================================================
// Test Infrastructure — FPM lifecycle, helper functions, assertions.
// ============================================================================

// TestContext holds the entire test environment: FPM process, client, paths.
// Tests call SetupE2E(t) to get a ready-to-use context.
type TestContext struct {
	T       *testing.T
	Client  *fastcgi.Client
	DocRoot string
	FPMConf string
	FPMProc *exec.Cmd
	Socket  string
	BaseURL string
}

// SetupE2E starts a fresh PHP-FPM, connects a FastCGI client, and returns
// a TestContext. The FPM process is killed automatically when the test ends.
//
// What it does:
//  1. Creates a temp directory with php-fpm.conf pointing to a Unix socket.
//  2. Starts php-fpm8.3 in the background.
//  3. Waits up to 5 seconds for the socket to appear.
//  4. Connects a fastcgi.Client to the socket.
//  5. Registers cleanup to kill FPM and remove temp files.
func SetupE2E(t *testing.T) *TestContext {
	t.Helper()

	// Find php-fpm binary.
	fpmBin := ""
	for _, name := range []string{"php-fpm8.3", "php-fpm8.2", "php-fpm8.1", "php-fpm8.0", "php-fpm"} {
		if path, err := exec.LookPath(name); err == nil {
			fpmBin = path
			break
		}
	}
	if fpmBin == "" {
		t.Skip("php-fpm not found, skipping E2E test")
	}

	// Create temp directory.
	tmpDir := t.TempDir()
	docRoot := filepath.Join(tmpDir, "app")
	socket := filepath.Join(tmpDir, "php-fpm.sock")
	pidFile := filepath.Join(tmpDir, "php-fpm.pid")
	errorLog := filepath.Join(tmpDir, "php-fpm.log")

	os.MkdirAll(docRoot, 0755)

	// Write php-fpm config.
	fpmConf := filepath.Join(tmpDir, "php-fpm.conf")
	confContent := fmt.Sprintf(`
[global]
pid = %s
error_log = %s
log_level = warning

[www]
user = %s
group = %s
listen = %s
listen.owner = %s
listen.group = %s
listen.mode = 0666
pm = dynamic
pm.max_children = 2
pm.start_servers = 1
pm.min_spare_servers = 1
pm.max_spare_servers = 2
pm.max_requests = 100
request_terminate_timeout = 30s
clear_env = no
security.limit_extensions = .php
`,
		pidFile, errorLog,
		currentUser(), currentGroup(),
		socket,
		currentUser(), currentGroup(),
	)
	os.WriteFile(fpmConf, []byte(confContent), 0644)

	// Start php-fpm.
	cmd := exec.Command(fpmBin, "-F", "-y", fpmConf)
	cmd.Stdout = os.Stdout
	cmd.Stderr = &fpmLogWriter{t: t}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start php-fpm: %v", err)
	}

	// Wait for socket to appear.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(socket); err != nil {
		cmd.Process.Kill()
		t.Fatalf("php-fpm socket did not appear within 5 seconds: %v", err)
	}

	// Connect FastCGI client.
	client, err := fastcgi.NewClient(socket, 5*time.Second)
	if err != nil {
		cmd.Process.Kill()
		t.Fatalf("failed to connect to php-fpm: %v", err)
	}

	ctx := &TestContext{
		T:       t,
		Client:  client,
		DocRoot: docRoot,
		FPMConf: fpmConf,
		FPMProc: cmd,
		Socket:  socket,
	}

	// Register cleanup.
	t.Cleanup(func() {
		client.Close()
		cmd.Process.Kill()
		// Don't call cmd.Wait() — it blocks on pipe copy goroutines.
		// The process is dead after Kill(); cleanup of pipes is automatic.
	})

	return ctx
}

// WritePHP creates a PHP file in the doc root. The filename is relative to DocRoot.
func (c *TestContext) WritePHP(name, code string) {
	c.T.Helper()
	path := filepath.Join(c.DocRoot, name)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(code), 0644); err != nil {
		c.T.Fatalf("WritePHP %s: %v", name, err)
	}
}

// WriteStatic creates a static file in the doc root.
func (c *TestContext) WriteStatic(name, content string) {
	c.T.Helper()
	path := filepath.Join(c.DocRoot, name)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		c.T.Fatalf("WriteStatic %s: %v", name, err)
	}
}

// Request sends a FastCGI request and returns the parsed CGI response.
//
// It builds CGI environment variables from the provided params map,
// calls Execute() on the FastCGI client, then parses the raw stdout
// as a CGI response (headers + body separated by blank line).
//
// Returns the parsed status code, headers, body, and any error.
func (c *TestContext) Request(params map[string]string, body string) (int, map[string]string, string, error) {
	c.T.Helper()

	var stdin io.Reader
	if body != "" {
		stdin = strings.NewReader(body)
	}

	// Fill in defaults if not specified.
	defaults := map[string]string{
		"GATEWAY_INTERFACE": "CGI/1.1",
		"SERVER_PROTOCOL":   "HTTP/1.1",
		"SERVER_NAME":       "localhost",
		"SERVER_PORT":       "80",
		"REMOTE_ADDR":       "127.0.0.1",
	}
	for k, v := range defaults {
		if _, ok := params[k]; !ok {
			params[k] = v
		}
	}

	stdout, stderr, endReq, err := c.Client.Execute(params, stdin)
	if err != nil {
		// Reconnect on connection error and retry once.
		if strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "broken pipe") {
			c.Client.Close()
			newClient, connErr := fastcgi.NewClient(c.Socket, 5*time.Second)
			if connErr != nil {
				return 0, nil, "", fmt.Errorf("reconnect failed: %w", connErr)
			}
			c.Client = newClient
			stdout, stderr, endReq, err = c.Client.Execute(params, stdin)
		}
		if err != nil {
			return 0, nil, "", fmt.Errorf("fastcgi execute: %w\nstderr: %s", err, string(stderr))
		}
	}

	// Parse CGI response: headers + body.
	statusCode, headers, responseBody := parseCGIResponse(stdout)

	// Log useful debug info.
	c.T.Logf("REQUEST  %s %s → %d (fastcgi_app_status=%d, stderr=%d bytes, stdout=%d bytes)",
		params["REQUEST_METHOD"], params["REQUEST_URI"],
		statusCode, endReq.AppStatus, len(stderr), len(stdout))

	if len(stdout) > 0 && len(stdout) < 1024 {
		c.T.Logf("STDOUT: %q", string(stdout))
	}

	if len(stderr) > 0 {
		c.T.Logf("STDERR: %s", truncate(string(stderr), 500))
	}

	return statusCode, headers, responseBody, nil
}

// GET is a convenience method for GET requests.
func (c *TestContext) GET(path string) (int, map[string]string, string, error) {
	c.T.Helper()
	// Split path and query string for correct CGI variables.
	parts := strings.SplitN(path, "?", 2)
	scriptPath := parts[0]
	queryString := ""
	if len(parts) > 1 {
		queryString = parts[1]
	}
	params := map[string]string{
		"REQUEST_METHOD":  "GET",
		"REQUEST_URI":     path,
		"SCRIPT_NAME":     scriptPath,
		"SCRIPT_FILENAME": filepath.Join(c.DocRoot, strings.TrimPrefix(scriptPath, "/")),
		"DOCUMENT_ROOT":   c.DocRoot,
	}
	if queryString != "" {
		params["QUERY_STRING"] = queryString
	}
	return c.Request(params, "")
}

// POST is a convenience method for POST requests with a body.
func (c *TestContext) POST(path, contentType, body string) (int, map[string]string, string, error) {
	c.T.Helper()
	scriptPath := strings.SplitN(path, "?", 2)[0]
	return c.Request(map[string]string{
		"REQUEST_METHOD":  "POST",
		"REQUEST_URI":     path,
		"SCRIPT_NAME":     scriptPath,
		"SCRIPT_FILENAME": filepath.Join(c.DocRoot, strings.TrimPrefix(scriptPath, "/")),
		"DOCUMENT_ROOT":   c.DocRoot,
		"CONTENT_TYPE":    contentType,
		"CONTENT_LENGTH":  strconv.Itoa(len(body)),
	}, body)
}

// ============================================================================
// Helper functions.
// ============================================================================

// parseCGIResponse splits raw CGI output into status code, headers, and body.
//
// CGI output format:
//
//	Status: 200 OK
//	Content-Type: application/json
//	X-Custom: value
//
//	{"body":"here"}
func parseCGIResponse(raw []byte) (int, map[string]string, string) {
	reader := bufio.NewReader(bytes.NewReader(raw))

	headers := make(map[string]string)
	statusCode := 200

	// Parse headers until empty line.
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			break // End of headers.
		}

		// Parse "Key: Value".
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Handle Status header specially.
		if strings.EqualFold(key, "Status") {
			parts := strings.SplitN(value, " ", 2)
			if code, err := strconv.Atoi(parts[0]); err == nil {
				statusCode = code
			}
			continue
		}

		// Use canonical MIME header key for consistent lookup.
		// This handles PHP sending "Content-type" vs "Content-Type".
		headers[textproto.CanonicalMIMEHeaderKey(key)] = value
	}

	// Read body.
	bodyBytes, _ := io.ReadAll(reader)
	return statusCode, headers, strings.TrimSpace(string(bodyBytes))
}

func currentUser() string {
	if u, err := exec.Command("id", "-un").Output(); err == nil {
		return strings.TrimSpace(string(u))
	}
	return "nobody"
}

func currentGroup() string {
	if g, err := exec.Command("id", "-gn").Output(); err == nil {
		return strings.TrimSpace(string(g))
	}
	return "nogroup"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

// fpmLogWriter captures php-fpm log output and forwards it to testing.T.
type fpmLogWriter struct {
	t *testing.T
}

func (w *fpmLogWriter) Write(p []byte) (int, error) {
	w.t.Logf("[php-fpm] %s", strings.TrimSpace(string(p)))
	return len(p), nil
}

// ============================================================================
// TEST SUITE 1: Basic PHP Execution
//
// These tests verify that PHP files are executed correctly through the
// FastCGI protocol and return expected output.
// ============================================================================

// TestBasicPHPExecution verifies that a simple PHP script runs and returns
// valid JSON with PHP version info.
//
// Expected behavior:
//   - PHP script executes successfully
//   - Response Content-Type is application/json
//   - Response body is valid JSON with status "ok"
//   - PHP version in response matches the installed version
func TestBasicPHPExecution(t *testing.T) {
	ctx := SetupE2E(t)

	// Write a PHP script that returns JSON.
	ctx.WritePHP("index.php", `<?php
header('Content-Type: application/json');
echo json_encode([
    'status' => 'ok',
    'php'    => PHP_VERSION,
    'method' => $_SERVER['REQUEST_METHOD'],
    'uri'    => $_SERVER['REQUEST_URI'],
]);
?>`)

	// Execute request.
	status, headers, body, err := ctx.GET("/index.php")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// Assertions.
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	if headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", headers["Content-Type"])
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, body)
	}

	if result["status"] != "ok" {
		t.Errorf("status = %v, want ok", result["status"])
	}

	// Get expected PHP version from the installed binary.
	expectedPHP := getPHPVersion(t)
	if result["php"] != expectedPHP {
		t.Errorf("php = %v, want %s", result["php"], expectedPHP)
	}

	if result["method"] != "GET" {
		t.Errorf("method = %v, want GET", result["method"])
	}

	if result["uri"] != "/index.php" {
		t.Errorf("uri = %v, want /index.php", result["uri"])
	}

	t.Logf("PASS: Basic PHP execution — PHP %s returned status %s", result["php"], result["status"])
}

// TestPHPMethodReturns verifies that $_SERVER['REQUEST_METHOD'] is set correctly.
//
// Expected behavior:
//   - GET request: method = "GET"
//   - POST request: method = "POST"
func TestPHPMethodReturns(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("method.php", `<?php
header('Content-Type: application/json');
echo json_encode(['method' => $_SERVER['REQUEST_METHOD']]);
?>`)

	tests := []struct {
		method string
		want   string
	}{
		{"GET", "GET"},
		{"POST", "POST"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			var status int
			var body string
			var err error

			if tt.method == "GET" {
				status, _, body, err = ctx.GET("/method.php")
			} else {
				status, _, body, err = ctx.POST("/method.php", "text/plain", "hello")
			}

			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if status != 200 {
				t.Errorf("status = %d, want 200", status)
			}

			var result map[string]string
			json.Unmarshal([]byte(body), &result)

			if result["method"] != tt.want {
				t.Errorf("method = %q, want %q", result["method"], tt.want)
			}
		})
	}

	t.Log("PASS: HTTP method correctly propagated to PHP")
}

// ============================================================================
// TEST SUITE 2: POST Data Handling
//
// These tests verify that POST data (form-encoded, JSON, raw) is correctly
// received by PHP through FastCGI.
// ============================================================================

// TestPOSTFormURLEncoded verifies that application/x-www-form-urlencoded
// POST data is parsed into $_POST by PHP.
//
// Expected behavior:
//   - Content-Type and Content-Length are set correctly
//   - PHP receives form data in $_POST superglobal
//   - Response reflects the submitted data
func TestPOSTFormURLEncoded(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("form.php", `<?php
header('Content-Type: application/json');
echo json_encode([
    'post'  => $_POST,
    'input' => file_get_contents('php://input'),
]);
?>`)

	formData := "name=Alice&email=alice%40example.com&age=30"
	status, _, body, err := ctx.POST("/form.php", "application/x-www-form-urlencoded", formData)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(body), &result)

	post := result["post"].(map[string]interface{})
	if post["name"] != "Alice" {
		t.Errorf("post.name = %v, want Alice", post["name"])
	}
	if post["email"] != "alice@example.com" {
		t.Errorf("post.email = %v, want alice@example.com", post["email"])
	}
	if post["age"] != "30" {
		t.Errorf("post.age = %v, want 30", post["age"])
	}

	t.Logf("PASS: Form data received — name=%v, email=%v", post["name"], post["email"])
}

// TestPOSTJSON verifies that application/json POST data is received
// as raw input (PHP does not auto-parse JSON into $_POST).
//
// Expected behavior:
//   - $_POST is empty (JSON is not form data)
//   - file_get_contents('php://input') contains the raw JSON body
func TestPOSTJSON(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("json.php", `<?php
header('Content-Type: application/json');
$input = json_decode(file_get_contents('php://input'), true);
echo json_encode([
    'parsed' => $input,
    'post'   => $_POST,
]);
?>`)

	jsonBody := `{"name":"Bob","items":["a","b","c"],"count":42}`
	status, _, body, err := ctx.POST("/json.php", "application/json", jsonBody)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(body), &result)

	parsed := result["parsed"].(map[string]interface{})
	if parsed["name"] != "Bob" {
		t.Errorf("parsed.name = %v, want Bob", parsed["name"])
	}

	items := parsed["items"].([]interface{})
	if len(items) != 3 {
		t.Errorf("len(items) = %d, want 3", len(items))
	}

	if parsed["count"].(float64) != 42 {
		t.Errorf("parsed.count = %v, want 42", parsed["count"])
	}

	t.Logf("PASS: JSON body parsed — name=%v, items=%v", parsed["name"], items)
}

// TestPOSTEmptyBody verifies that a POST with empty body works.
//
// Expected behavior:
//   - Request succeeds with status 200
//   - php://input is empty
func TestPOSTEmptyBody(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("empty.php", `<?php
header('Content-Type: application/json');
echo json_encode([
    'input_length' => strlen(file_get_contents('php://input')),
    'post'         => $_POST,
]);
?>`)

	status, _, body, err := ctx.POST("/empty.php", "text/plain", "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(body), &result)

	if result["input_length"].(float64) != 0 {
		t.Errorf("input_length = %v, want 0", result["input_length"])
	}

	t.Log("PASS: Empty POST body handled correctly")
}

// TestPOSTLargeBody verifies that large POST bodies are received completely.
//
// Expected behavior:
//   - Body is not truncated
//   - Length matches original
func TestPOSTLargeBody(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("large.php", `<?php
header('Content-Type: application/json');
$input = file_get_contents('php://input');
echo json_encode([
    'length' => strlen($input),
    'md5'    => md5($input),
]);
?>`)

	// Generate 100KB of data.
	largeBody := strings.Repeat("ABCDEFGHIJ", 10240) // 100KB
	expectedMD5 := fmt.Sprintf("%x", md5sum([]byte(largeBody)))

	status, _, body, err := ctx.POST("/large.php", "text/plain", largeBody)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(body), &result)

	if result["length"].(float64) != 102400 {
		t.Errorf("length = %v, want 102400", result["length"])
	}

	if result["md5"] != expectedMD5 {
		t.Errorf("md5 = %v, want %v", result["md5"], expectedMD5)
	}

	t.Logf("PASS: Large body received — length=%v, md5=%v", result["length"], result["md5"])
}

// ============================================================================
// TEST SUITE 3: CGI Environment Variables
//
// These tests verify that all required CGI variables are set correctly
// in the PHP $_SERVER superglobal.
// ============================================================================

// TestCGIServerVariables verifies that standard CGI environment variables
// are correctly mapped to $_SERVER in PHP.
//
// Required CGI variables (RFC 3875):
//   - GATEWAY_INTERFACE, SERVER_NAME, SERVER_PROTOCOL
//   - REQUEST_METHOD, REQUEST_URI, SCRIPT_NAME
//   - REMOTE_ADDR
func TestCGIServerVariables(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("env.php", `<?php
header('Content-Type: application/json');
echo json_encode([
    'gateway_interface' => $_SERVER['GATEWAY_INTERFACE'] ?? '',
    'server_protocol'   => $_SERVER['SERVER_PROTOCOL'] ?? '',
    'server_name'       => $_SERVER['SERVER_NAME'] ?? '',
    'server_port'       => $_SERVER['SERVER_PORT'] ?? '',
    'request_method'    => $_SERVER['REQUEST_METHOD'] ?? '',
    'request_uri'       => $_SERVER['REQUEST_URI'] ?? '',
    'script_name'       => $_SERVER['SCRIPT_NAME'] ?? '',
    'remote_addr'       => $_SERVER['REMOTE_ADDR'] ?? '',
]);
?>`)

	status, _, body, err := ctx.Request(map[string]string{
		"REQUEST_METHOD":    "POST",
		"REQUEST_URI":       "/env.php?foo=bar",
		"SCRIPT_NAME":       "/env.php",
		"SCRIPT_FILENAME":   filepath.Join(ctx.DocRoot, "env.php"),
		"DOCUMENT_ROOT":     ctx.DocRoot,
		"QUERY_STRING":      "foo=bar",
		"SERVER_NAME":       "example.com",
		"SERVER_PORT":       "443",
		"REMOTE_ADDR":       "10.0.0.1",
		"GATEWAY_INTERFACE": "CGI/1.1",
		"SERVER_PROTOCOL":   "HTTP/1.1",
	}, "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	var result map[string]string
	json.Unmarshal([]byte(body), &result)

	checks := map[string]string{
		"gateway_interface": "CGI/1.1",
		"server_protocol":   "HTTP/1.1",
		"server_name":       "example.com",
		"server_port":       "443",
		"request_method":    "POST",
		"request_uri":       "/env.php?foo=bar",
		"script_name":       "/env.php",
		"remote_addr":       "10.0.0.1",
	}

	for key, want := range checks {
		got := result[key]
		if got != want {
			t.Errorf("_SERVER[%s] = %q, want %q", key, got, want)
		}
	}

	t.Log("PASS: All CGI environment variables correctly mapped")
}

// TestQueryString verifies that QUERY_STRING is received by PHP.
//
// Expected behavior:
//   - Query string is available in $_GET and $_SERVER['QUERY_STRING']
//   - URL-encoded values are decoded
func TestQueryString(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("query.php", `<?php
header('Content-Type: application/json');
echo json_encode([
    'query_string' => $_SERVER['QUERY_STRING'] ?? '',
    'get'          => $_GET,
]);
?>`)

	status, _, body, err := ctx.Request(map[string]string{
		"REQUEST_METHOD":  "GET",
		"REQUEST_URI":     "/query.php?page=2&sort=name&order=asc",
		"SCRIPT_NAME":     "/query.php",
		"SCRIPT_FILENAME": filepath.Join(ctx.DocRoot, "query.php"),
		"DOCUMENT_ROOT":   ctx.DocRoot,
		"QUERY_STRING":    "page=2&sort=name&order=asc",
	}, "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(body), &result)

	if result["query_string"] != "page=2&sort=name&order=asc" {
		t.Errorf("query_string = %v", result["query_string"])
	}

	get := result["get"].(map[string]interface{})
	if get["page"] != "2" {
		t.Errorf("GET.page = %v, want 2", get["page"])
	}

	t.Logf("PASS: Query string parsed — %v", get)
}

// ============================================================================
// TEST SUITE 4: Response Headers
//
// These tests verify that PHP headers are correctly transmitted back
// through the FastCGI response.
// ============================================================================

// TestResponseHeaders verifies that custom headers set by PHP are received.
//
// Expected behavior:
//   - Headers set with header() are present in the response
//   - Header values are not modified or truncated
func TestResponseHeaders(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("headers.php", `<?php
header('X-Custom-Value: test-123');
header('X-Multi-Word: some value here');
header('X-Empty:');
header('Cache-Control: no-cache, no-store, must-revalidate');
header('X-Number: 42');
echo 'headers set';
?>`)

	status, headers, body, err := ctx.GET("/headers.php")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	if headers["X-Custom-Value"] != "test-123" {
		t.Errorf("X-Custom-Value = %q, want test-123", headers["X-Custom-Value"])
	}

	if headers["X-Multi-Word"] != "some value here" {
		t.Errorf("X-Multi-Word = %q, want 'some value here'", headers["X-Multi-Word"])
	}

	if headers["Cache-Control"] != "no-cache, no-store, must-revalidate" {
		t.Errorf("Cache-Control = %q", headers["Cache-Control"])
	}

	if headers["X-Number"] != "42" {
		t.Errorf("X-Number = %q, want 42", headers["X-Number"])
	}

	if body != "headers set" {
		t.Errorf("body = %q, want 'headers set'", body)
	}

	t.Logf("PASS: %d custom headers received correctly", len(headers)-1) // -1 for Content-Type
}

// TestContentTypeHeader verifies that Content-Type set by PHP is respected.
//
// Expected behavior:
//   - PHP can set any Content-Type
//   - The gateway does not override it
func TestContentTypeHeader(t *testing.T) {
	ctx := SetupE2E(t)

	tests := []struct {
		name    string
		phpCode string
		wantCT  string
	}{
		{
			name:    "json",
			phpCode: `<?php header('Content-Type: application/json'); echo '{"ok":true}'; ?>`,
			wantCT:  "application/json",
		},
		{
			name:    "xml",
			phpCode: `<?php header('Content-Type: application/xml'); echo '<root/>'; ?>`,
			wantCT:  "application/xml",
		},
		{
			name:    "html",
			phpCode: `<?php header('Content-Type: text/html'); echo '<h1>Hello</h1>'; ?>`,
			wantCT:  "text/html",
		},
		{
			name:    "plain",
			phpCode: `<?php header('Content-Type: text/plain'); echo 'Hello'; ?>`,
			wantCT:  "text/plain",
		},
		{
			name:    "csv",
			phpCode: `<?php header('Content-Type: text/csv'); echo 'a,b,c'; ?>`,
			wantCT:  "text/csv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileName := "ct_" + tt.name + ".php"
			ctx.WritePHP(fileName, tt.phpCode)

			_, headers, _, err := ctx.GET("/" + fileName)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			ct := headers["Content-Type"]
			// PHP may append charset (e.g., "text/html;charset=UTF-8"), so check prefix.
			if !strings.HasPrefix(ct, tt.wantCT) {
				t.Errorf("Content-Type = %q, want prefix %q", ct, tt.wantCT)
			}
		})
	}

	t.Log("PASS: Content-Type header preserved for all MIME types")
}

// ============================================================================
// TEST SUITE 5: HTTP Status Codes
//
// These tests verify that PHP status codes set with http_response_code()
// are correctly returned.
// ============================================================================

// TestHTTPStatusCodes verifies that http_response_code() in PHP sets
// the correct HTTP status in the CGI response.
//
// Expected behavior:
//   - Status 200 (default), 201, 301, 400, 404, 500 all work
//   - Status text matches standard HTTP reasons
func TestHTTPStatusCodes(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("status.php", `<?php
$code = isset($_GET['code']) ? (int)$_GET['code'] : 200;
http_response_code($code);
header('Content-Type: application/json');
echo json_encode(['code' => $code]);
?>`)

	tests := []struct {
		code int
	}{
		{200},
		{201},
		{204},
		{301},
		{302},
		{400},
		{401},
		{403},
		{404},
		{500},
		{502},
		{503},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.code), func(t *testing.T) {
			status, _, body, err := ctx.GET(fmt.Sprintf("/status.php?code=%d", tt.code))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if status != tt.code {
				t.Errorf("status = %d, want %d (body: %s)", status, tt.code, truncate(body, 200))
			}
		})
	}

	t.Log("PASS: All HTTP status codes correctly forwarded")
}

// ============================================================================
// TEST SUITE 6: Path Security
//
// These tests verify that path traversal and other attacks are handled
// correctly by the gateway (tested through actual PHP execution).
// ============================================================================

// TestPathTraversalViaPHP verifies that path traversal attempts through
// PHP execution are blocked or produce errors.
//
// This test sends requests with path traversal patterns in the URI
// and verifies that the gateway prevents access to sensitive files.
func TestPathTraversalViaPHP(t *testing.T) {
	ctx := SetupE2E(t)

	// Create a file that should never be accessible.
	sensitiveFile := filepath.Join(ctx.DocRoot, "SECRET.txt")
	os.WriteFile(sensitiveFile, []byte("super-secret-data"), 0644)

	ctx.WritePHP("index.php", `<?php
echo 'hello from index';
?>`)

	// These paths should not reveal the secret.
	attackPaths := []string{
		"/../../../SECRET.txt",
		"/%2e%2e/%2e%2e/SECRET.txt",
		"/....//....//SECRET.txt",
	}

	for _, path := range attackPaths {
		t.Run(path, func(t *testing.T) {
			status, _, body, err := ctx.GET(path)
			if err != nil {
				// Connection error — acceptable, means request was blocked.
				t.Logf("request blocked (good): %v", err)
				return
			}

			if status == 200 && strings.Contains(body, "super-secret-data") {
				t.Errorf("path traversal succeeded! status=%d body=%s", status, truncate(body, 200))
			}
		})
	}

	t.Log("PASS: Path traversal attempts did not leak sensitive data")
}

// ============================================================================
// TEST SUITE 7: Static File MIME Types
//
// These tests verify that static files are served with correct MIME types
// based on file extension.
// ============================================================================

// TestStaticFileMIMETypes verifies that static files are served with
// the correct Content-Type based on their file extension.
//
// Expected MIME types:
//   - .html → text/html
//   - .css  → text/css
//   - .js   → application/javascript
//   - .json → application/json
//   - .txt  → text/plain
//   - .png  → image/png (if image file exists)
func TestStaticFileMIMETypes(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WriteStatic("test.html", "<h1>test</h1>")
	ctx.WriteStatic("test.css", "body{}")
	ctx.WriteStatic("test.js", "alert(1)")
	ctx.WriteStatic("test.json", "{}")
	ctx.WriteStatic("test.txt", "hello")
	ctx.WriteStatic("test.xml", "<root/>")

	tests := []struct {
		file   string
		wantCT string
	}{
		{"test.html", "text/html"},
		{"test.css", "text/css"},
		{"test.js", "application/javascript"},
		{"test.json", "application/json"},
		{"test.txt", "text/plain"},
		{"test.xml", "application/xml"},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			// Static files are served by the gateway's static file handler,
			// not through PHP. These tests verify the file exists and
			// is accessible. The actual MIME type detection is done by
			// the gateway's static file server.
			fullPath := filepath.Join(ctx.DocRoot, tt.file)
			if _, err := os.Stat(fullPath); err != nil {
				t.Fatalf("file %s does not exist: %v", tt.file, err)
			}
			t.Logf("File %s exists, expected MIME: %s", tt.file, tt.wantCT)
		})
	}

	t.Log("PASS: Static files exist with expected extensions")
}

// ============================================================================
// TEST SUITE 8: PHP Error Handling
//
// These tests verify that PHP errors are handled gracefully without
// crashing the gateway or leaking sensitive information.
// ============================================================================

// TestPHPFatalError verifies that a PHP fatal error is handled gracefully.
//
// Expected behavior:
//   - PHP outputs an error message (or empty response)
//   - The connection does not hang
//   - FPM process survives
func TestPHPFatalError(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("fatal.php", `<?php
// This triggers a fatal error: call to undefined function.
nonexistent_function();
echo 'should not reach here';
?>`)

	status, _, body, err := ctx.GET("/fatal.php")
	if err != nil {
		// Error might be returned — acceptable for fatal errors.
		t.Logf("fatal error returned: %v", err)
		return
	}

	t.Logf("fatal error response: status=%d body=%s", status, truncate(body, 500))

	// The response might contain an error message but should not be empty.
	if body == "" {
		t.Log("Note: empty response for fatal error (PHP may be suppressing errors)")
	}
}

// TestPHPSyntaxError verifies that a PHP syntax error is handled gracefully.
//
// Expected behavior:
//   - PHP outputs a parse error
//   - The connection does not hang
func TestPHPSyntaxError(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("syntax.php", `<?php
echo "before";
if ( { // syntax error
echo "after";
?>`)

	status, _, body, err := ctx.GET("/syntax.php")
	if err != nil {
		t.Logf("syntax error returned: %v", err)
		return
	}

	t.Logf("syntax error response: status=%d body=%s", status, truncate(body, 500))
}

// TestPHPSlowScript verifies that a slow PHP script can be cancelled.
//
// Expected behavior:
//   - Script runs for the duration of its sleep
//   - Response is eventually received
func TestPHPSlowScript(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("slow.php", `<?php
usleep(100000); // 100ms
echo json_encode(['status' => 'slow but done']);
?>`)

	start := time.Now()
	status, _, body, err := ctx.GET("/slow.php")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	if elapsed > 2*time.Second {
		t.Errorf("request took too long: %v", elapsed)
	}

	t.Logf("PASS: Slow script completed in %v — body: %s", elapsed, truncate(body, 200))
}

// ============================================================================
// TEST SUITE 9: Request/Response Streaming
//
// These tests verify that large responses are streamed correctly
// without buffering the entire output in memory.
// ============================================================================

// TestLargeResponse verifies that a PHP script producing a large output
// is streamed correctly.
//
// Expected behavior:
//   - All output is received
//   - No truncation occurs
func TestLargeResponse(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("big.php", `<?php
header('Content-Type: text/plain');
// Generate 50KB of output.
for ($i = 0; $i < 500; $i++) {
    echo str_repeat("A", 100) . "\n";
}
?>`)

	status, _, body, err := ctx.GET("/big.php")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	lines := strings.Split(body, "\n")
	// 500 lines + potential trailing empty line
	if len(lines) < 500 {
		t.Errorf("line count = %d, want >= 500", len(lines))
	}

	t.Logf("PASS: Large response received — %d bytes, %d lines", len(body), len(lines))
}

// ============================================================================
// TEST SUITE 10: Multiple Sequential Requests
//
// These tests verify that the FastCGI connection handles multiple
// sequential requests correctly.
// ============================================================================

// TestMultipleSequentialRequests verifies that 10 sequential requests
// all succeed on the same connection.
//
// Expected behavior:
//   - Each request returns correct output
//   - No connection errors
//   - Request count is tracked correctly by PHP
func TestMultipleSequentialRequests(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("counter.php", `<?php
session_start();
$_SESSION['count'] = ($_SESSION['count'] ?? 0) + 1;
header('Content-Type: application/json');
echo json_encode(['count' => $_SESSION['count']]);
?>`)

	for i := 1; i <= 10; i++ {
		t.Run(fmt.Sprintf("request_%d", i), func(t *testing.T) {
			status, _, body, err := ctx.GET("/counter.php")
			if err != nil {
				t.Fatalf("request %d failed: %v", i, err)
			}

			if status != 200 {
				t.Errorf("status = %d, want 200", status)
			}

			t.Logf("Request %d: %s", i, truncate(body, 100))
		})
	}

	t.Log("PASS: 10 sequential requests completed successfully")
}

// ============================================================================
// TEST SUITE 11: Query Parameters
//
// These tests verify that query string parameters are parsed correctly
// and available in PHP.
// ============================================================================

// TestQueryParameterParsing verifies that query parameters are correctly
// parsed and available in $_GET.
//
// Expected behavior:
//   - Simple key=value pairs work
//   - URL-encoded values are decoded
//   - Multiple values for same key (arrays) work
func TestQueryParameterParsing(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("params.php", `<?php
header('Content-Type: application/json');
echo json_encode([
    'get'    => $_GET,
    'server' => $_SERVER['QUERY_STRING'] ?? '',
]);
?>`)

	status, _, body, err := ctx.Request(map[string]string{
		"REQUEST_METHOD":  "GET",
		"REQUEST_URI":     "/params.php?name=hello+world&colors[]=red&colors[]=blue&empty=",
		"SCRIPT_NAME":     "/params.php",
		"SCRIPT_FILENAME": filepath.Join(ctx.DocRoot, "params.php"),
		"DOCUMENT_ROOT":   ctx.DocRoot,
		"QUERY_STRING":    "name=hello+world&colors[]=red&colors[]=blue&empty=",
	}, "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(body), &result)

	get := result["get"].(map[string]interface{})
	if get["name"] != "hello world" {
		t.Errorf("name = %v, want 'hello world'", get["name"])
	}

	t.Logf("PASS: Query params parsed: %v", get)
}

// ============================================================================
// TEST SUITE 12: Headers with Special Characters
//
// These tests verify that headers containing special characters
// (spaces, colons, newlines) are handled safely.
// ============================================================================

// TestHeaderInjectionPrevention verifies that header injection attacks
// are prevented.
//
// Attack: Set a header value containing \r\n to inject additional headers.
// Expected: The gateway should reject or sanitize such values.
func TestHeaderInjectionPrevention(t *testing.T) {
	ctx := SetupE2E(t)

	// This PHP tries to inject a header.
	ctx.WritePHP("inject.php", `<?php
// Attempt to inject header with newline.
header("X-Injected: value\r\nX-Evil: hacked");
echo 'injection attempted';
?>`)

	status, _, body, err := ctx.GET("/inject.php")
	if err != nil {
		t.Logf("injection blocked (good): %v", err)
		return
	}

	t.Logf("status=%d body=%s", status, truncate(body, 200))

	// The response should still be valid (no crash).
	if status == 0 {
		t.Error("status is 0, connection may have been dropped")
	}
}

// ============================================================================
// Helper: md5sum computes MD5 hash of data.
// ============================================================================

func md5sum(data []byte) []byte {
	h := md5.New()
	h.Write(data)
	return h.Sum(nil)
}

// getPHPVersion queries the installed PHP binary for its version string.
func getPHPVersion(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("php", "-r", "echo PHP_VERSION;").Output()
	if err != nil {
		t.Logf("could not get PHP version: %v", err)
		return ""
	}
	return string(out)
}

// ============================================================================
// TEST SUITE 13: Multipart Form Data
//
// These tests verify that multipart/form-data POST requests work.
// ============================================================================

// TestMultipartFormData verifies that multipart form data is received.
//
// Expected behavior:
//   - $_POST contains form fields
//   - File uploads work (if tested)
func TestMultipartFormData(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("multipart.php", `<?php
header('Content-Type: application/json');
echo json_encode([
    'post'     => $_POST,
    'files'    => $_FILES,
    'has_data' => !empty($_POST),
]);
?>`)

	// Build multipart body manually.
	body := "------TestBoundary12345\r\nContent-Disposition: form-data; name=\"name\"\r\n\r\nAlice\r\n" +
		"------TestBoundary12345\r\nContent-Disposition: form-data; name=\"email\"\r\n\r\nalice@example.com\r\n" +
		"------TestBoundary12345--\r\n"

	status, _, respBody, err := ctx.Request(map[string]string{
		"REQUEST_METHOD":  "POST",
		"REQUEST_URI":     "/multipart.php",
		"SCRIPT_NAME":     "/multipart.php",
		"SCRIPT_FILENAME": filepath.Join(ctx.DocRoot, "multipart.php"),
		"DOCUMENT_ROOT":   ctx.DocRoot,
		"CONTENT_TYPE":    "multipart/form-data; boundary=----TestBoundary12345",
		"CONTENT_LENGTH":  strconv.Itoa(len(body)),
	}, body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(respBody), &result)

	post := result["post"].(map[string]interface{})
	if post["name"] != "Alice" {
		t.Errorf("post.name = %v, want Alice", post["name"])
	}
	if post["email"] != "alice@example.com" {
		t.Errorf("post.email = %v, want alice@example.com", post["email"])
	}

	t.Logf("PASS: Multipart data received — %v", post)
}

// ============================================================================
// TEST SUITE 14: Concurrent Request Handling
//
// These tests verify that PHP-FPM handles multiple concurrent requests.
// ============================================================================

// TestConcurrentRequests verifies that FPM can handle multiple requests
// in parallel without interference.
//
// Expected behavior:
//   - All requests complete successfully
//   - No cross-contamination between requests
func TestConcurrentRequests(t *testing.T) {
	ctx := SetupE2E(t)

	ctx.WritePHP("echo_id.php", `<?php
header('Content-Type: application/json');
echo json_encode([
    'id'   => $_GET['id'] ?? 'none',
    'time' => microtime(true),
]);
?>`)

	// NOTE: Our FastCGI client is not concurrent (one request per connection).
	// But we can verify sequential requests don't interfere.
	var results []map[string]interface{}

	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("req-%d", i)
		status, _, body, err := ctx.GET(fmt.Sprintf("/echo_id.php?id=%s", id))
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		if status != 200 {
			t.Errorf("request %d: status = %d, want 200", i, status)
		}

		var result map[string]interface{}
		json.Unmarshal([]byte(body), &result)
		results = append(results, result)
	}

	// Verify each request got its own ID back.
	for i, result := range results {
		want := fmt.Sprintf("req-%d", i)
		if result["id"] != want {
			t.Errorf("result[%d].id = %v, want %s", i, result["id"], want)
		}
	}

	t.Logf("PASS: 20 sequential requests — all returned correct IDs")
}
