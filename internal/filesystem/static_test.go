package filesystem

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPrecompressedFileServer(t *testing.T) {
	dir := t.TempDir()

	// Create a normal file.
	os.WriteFile(filepath.Join(dir, "style.css"), []byte("body{}"), 0644)

	// Create a gzipped version.
	os.WriteFile(filepath.Join(dir, "style.css.gz"), []byte("gzip-data"), 0644)

	srv := &PrecompressedFileServer{Root: dir, Index: "index.html"}

	t.Run("plain", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/style.css", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Errorf("status = %d, want 200", w.Code)
		}
		if w.Header().Get("Content-Encoding") != "" {
			t.Error("should not set Content-Encoding without Accept-Encoding")
		}
	})

	t.Run("gzip", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/style.css", nil)
		r.Header.Set("Accept-Encoding", "gzip, deflate")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Errorf("status = %d, want 200", w.Code)
		}
		if w.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("Content-Encoding = %q, want %q", w.Header().Get("Content-Encoding"), "gzip")
		}
	})

	t.Run("not found", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/missing.css", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestPrecompressedFileServerIndex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0644)

	srv := &PrecompressedFileServer{Root: dir, Index: "index.html"}

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPrecompressedFileServerTraversal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0644)

	srv := &PrecompressedFileServer{Root: dir, Index: "index.html"}

	r := httptest.NewRequest("GET", "/../secret.txt", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400 (traversal should be blocked)", w.Code)
	}
}

func TestPrecompressedFileServerDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "index.html"), []byte("dir index"), 0644)

	srv := &PrecompressedFileServer{Root: dir, Index: "index.html"}

	r := httptest.NewRequest("GET", "/sub/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPrecompressedFileServerBrotli(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("js-data"), 0644)
	os.WriteFile(filepath.Join(dir, "app.js.br"), []byte("br-data"), 0644)

	srv := &PrecompressedFileServer{Root: dir, Index: "index.html"}

	r := httptest.NewRequest("GET", "/app.js", nil)
	r.Header.Set("Accept-Encoding", "br")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Header().Get("Content-Encoding") != "br" {
		t.Errorf("Content-Encoding = %q, want %q", w.Header().Get("Content-Encoding"), "br")
	}
}

func TestPrecompressedFileServerHEAD(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

	srv := &PrecompressedFileServer{Root: dir, Index: "index.html"}

	r := httptest.NewRequest(http.MethodHead, "/test.txt", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
