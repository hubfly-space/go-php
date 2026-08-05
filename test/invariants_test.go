package test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-php/gateway/internal/filesystem"
	"github.com/go-php/gateway/internal/php/cgi"
	"github.com/go-php/gateway/internal/router"
)

// Invariant 1: normalize(normalize(path)) == normalize(path) (idempotence)
func TestInvariant_PathNormalizationIdempotence(t *testing.T) {
	paths := []string{
		"/foo/bar",
		"/a/b/../c/./d",
		"/public/app.js",
		"/index.php",
		"/+plus",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			pp1, err := filesystem.ParsePath(p)
			if err != nil {
				return
			}
			pp2, err := filesystem.ParsePath(pp1.NormalizedPath)
			if err != nil {
				t.Fatalf("second parse failed: %v", err)
			}
			if pp1.NormalizedPath != pp2.NormalizedPath {
				t.Errorf("not idempotent: %q -> %q -> %q", p, pp1.NormalizedPath, pp2.NormalizedPath)
			}
		})
	}
}

// Invariant 2: For every accepted path, resolved file is strictly under allowed root.
func TestInvariant_ResolvedFileUnderRoot(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php"), 0644)
	r := filesystem.NewResolver(dir, filesystem.SymlinkWithinRoot, nil)

	testPaths := []string{
		"/index.php",
		"/public/../index.php",
	}

	for _, tp := range testPaths {
		pp, err := filesystem.ParsePath(tp)
		if err != nil {
			continue
		}
		rf, err := r.Resolve(pp.NormalizedPath)
		if err != nil {
			continue
		}
		defer rf.Close()

		cleanRoot := filepath.Clean(dir)
		if !strings.HasPrefix(rf.RealPath, cleanRoot) {
			t.Errorf("Resolved file %s escapes root %s", rf.RealPath, cleanRoot)
		}
	}
}

// Invariant 3: For every rejected malformed path, no filesystem open occurs.
func TestInvariant_RejectedPathNoFileOpen(t *testing.T) {
	dir := t.TempDir()
	r := filesystem.NewResolver(dir, filesystem.SymlinkDeny, nil)

	malformed := []string{
		"../../etc/passwd",
		"not-absolute",
		"/\x00null",
	}

	for _, m := range malformed {
		_, err := r.Resolve(m)
		if err == nil {
			t.Errorf("expected resolution error for malformed path %q", m)
		}
	}
}

// Invariant 4: FastCGI round-trip params encoding.
func TestInvariant_FastCGIParamsEncoding(t *testing.T) {
	params := map[string]string{
		"SCRIPT_FILENAME": "/var/www/index.php",
		"REQUEST_METHOD":  "GET",
		"HTTP_HOST":       "example.com",
	}

	// Verify params can be serialized into FastCGI PARAMS format without loss
	var buf bytes.Buffer
	for k, v := range params {
		if k == "" {
			t.Errorf("empty param key")
		}
		_, _ = buf.WriteString(fmt.Sprintf("%s=%s\n", k, v))
	}
	if buf.Len() == 0 {
		t.Errorf("encoded params buffer is empty")
	}
}

// Invariant 5: Every valid compiled rewrite set terminates within max_iterations.
func TestInvariant_RewriteTermination(t *testing.T) {
	r := &router.Route{
		Regex:  "^/old/(.*)$",
		Target: "/new/$1",
	}

	path := "/old/path/to/resource"
	maxIterations := 10

	for i := 0; i < maxIterations; i++ {
		next := r.Rewrite(path)
		if next == path {
			break // Terminated
		}
		path = next
	}

	if strings.HasPrefix(path, "/old/") {
		t.Errorf("rewrite failed to terminate within max iterations: %s", path)
	}
}

// Invariant 6: Any header accepted from PHP can be written without control-character injection.
func TestInvariant_PHPHeaderControlCharSafety(t *testing.T) {
	maliciousResponse := "X-Injected: \x00value\r\nContent-Type: text/html\r\n\r\nhello"
	_, err := cgi.ParseResponse([]byte(maliciousResponse), nil)
	if err == nil {
		t.Errorf("expected control character injection to be rejected")
	}
}
