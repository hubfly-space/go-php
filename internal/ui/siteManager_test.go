package ui

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newTestSiteHandler(t *testing.T) (*siteHandler, string) {
	t.Helper()

	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	write("index.html", "<h1>home</h1>")
	write("public.txt", "public")
	write(".env", "DB_PASSWORD=hunter2")
	write(".git/config", "[core]")
	write("db.sqlite", "sqlite-data")
	write("backup.sql", "DROP TABLE users;")

	// Something outside the webroot for traversal attempts to aim at.
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("top secret"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	return newSiteHandler(root, "test-site", "", slog.New(slog.NewTextHandler(io.Discard, nil))), root
}

func TestSiteHandlerServesOrdinaryFiles(t *testing.T) {
	h, _ := newTestSiteHandler(t)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/public.txt", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "public" {
		t.Errorf("body = %q, want %q", w.Body.String(), "public")
	}
}

func TestSiteHandlerBlocksProtectedPatterns(t *testing.T) {
	// The site handler previously had no protected-pattern check at all, so
	// .env under a site webroot was served in full.
	h, _ := newTestSiteHandler(t)

	for _, path := range []string{
		"/.env",
		"/.git/config",
		"/db.sqlite",
		"/backup.sql",
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))

		if w.Code == http.StatusOK {
			t.Errorf("%s was served with status 200; body=%q", path, w.Body.String())
		}
	}
}

func TestSiteHandlerBlocksTraversal(t *testing.T) {
	h, _ := newTestSiteHandler(t)

	for _, path := range []string{
		"/../secret.txt",
		"/../../etc/passwd",
		"/..%2f..%2fetc%2fpasswd",
		"/%2e%2e/%2e%2e/etc/passwd",
		"/....//....//etc/passwd",
		"/\\..\\..\\etc\\passwd",
	} {
		req := httptest.NewRequest("GET", "http://example.test"+path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Errorf("traversal %q returned 200; body=%q", path, w.Body.String())
		}
	}
}

func TestSiteHandlerServesDirectoryIndex(t *testing.T) {
	h, _ := newTestSiteHandler(t)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "<h1>home</h1>" {
		t.Errorf("body = %q, want the index", w.Body.String())
	}
}

func TestSiteHandlerMissingFileIs404(t *testing.T) {
	h, _ := newTestSiteHandler(t)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/nope.txt", nil))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
