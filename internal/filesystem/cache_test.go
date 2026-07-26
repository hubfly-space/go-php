package filesystem

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheControlledFileServer_DefaultHeaders(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "style.css"), []byte(`body {}`), 0644)

	srv := NewCacheControlledFileServer(dir, "index.html", nil)

	req := httptest.NewRequest("GET", "/style.css", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	cc := rec.Header().Get("Cache-Control")
	if cc == "" {
		t.Error("expected Cache-Control header")
	}
	t.Logf("Cache-Control: %s", cc)
}

func TestCacheControlledFileServer_NoCache(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "api"), 0755)
	os.WriteFile(filepath.Join(dir, "api", "data.json"), []byte(`{"ok":true}`), 0644)

	srv := NewCacheControlledFileServer(dir, "index.html", nil)

	req := httptest.NewRequest("GET", "/api/data.json", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	cc := rec.Header().Get("Cache-Control")
	if cc != "no-cache, no-store, must-revalidate" {
		t.Errorf("expected no-cache, got %s", cc)
	}
}

func TestCacheControlledFileServer_Immutable(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "assets"), 0755)
	os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte(`console.log("hi")`), 0644)

	srv := NewCacheControlledFileServer(dir, "index.html", nil)

	req := httptest.NewRequest("GET", "/assets/app.js", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	cc := rec.Header().Get("Cache-Control")
	if cc != "public, max-age=31536000, immutable" {
		t.Errorf("expected immutable, got %s", cc)
	}
}

func TestCacheControlledFileServer_ETag(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte(`hello`), 0644)

	srv := NewCacheControlledFileServer(dir, "index.html", nil)

	req := httptest.NewRequest("GET", "/file.txt", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Error("expected ETag header")
	}
	t.Logf("ETag: %s", etag)

	// Test If-None-Match.
	req2 := httptest.NewRequest("GET", "/file.txt", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()

	srv.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusNotModified {
		t.Errorf("expected 304 for matching ETag, got %d", rec2.Code)
	}
}

func TestCacheControlledFileServer_CustomPolicy(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.css"), []byte(`body {}`), 0644)

	policy := &CachePolicy{
		DefaultMaxAge:  2 * time.Hour,
		ImmutablePaths: []string{"/v2/"},
		NoCachePaths:   []string{"/dynamic/"},
		ETag:           false,
	}

	srv := NewCacheControlledFileServer(dir, "index.html", policy)

	req := httptest.NewRequest("GET", "/test.css", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	cc := rec.Header().Get("Cache-Control")
	if cc != "public, max-age=7200" {
		t.Errorf("expected 7200s cache, got %s", cc)
	}

	// ETag should be disabled.
	etag := rec.Header().Get("ETag")
	if etag != "" {
		t.Error("expected no ETag when disabled")
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{999, "999"},
		{-1, "-1"},
		{-42, "-42"},
	}
	for _, tt := range tests {
		result := itoa(tt.input)
		if result != tt.expected {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
