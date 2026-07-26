package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create test files.
	writeFile(t, filepath.Join(dir, "index.php"), "<?php echo 'hello';")
	writeFile(t, filepath.Join(dir, "style.css"), "body{}")
	writeFile(t, filepath.Join(dir, ".env"), "SECRET=1")
	writeFile(t, filepath.Join(dir, ".git", "config"), "[core]")
	writeFile(t, filepath.Join(dir, "secret.sql"), "SELECT 1")
	writeFile(t, filepath.Join(dir, "public", "app.js"), "console.log()")

	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolver_ResolveNormal(t *testing.T) {
	dir := setupTestDir(t)
	r := NewResolver(dir, SymlinkDeny, DefaultProtectedPatterns())

	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{"index.php", "/index.php", nil},
		{"style.css", "/style.css", nil},
		{"subdir file", "/public/app.js", nil},
		{"missing", "/missing.html", ErrFileNotFound},
		{"protected .env", "/.env", ErrProtectedFile},
		{"protected .git", "/.git/config", ErrProtectedFile},
		{"protected sql", "/secret.sql", ErrProtectedFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rf, err := r.Resolve(tt.path)
			if tt.wantErr != nil {
				if err == nil {
					rf.Close()
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if err != tt.wantErr {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer rf.Close()
			if rf.RealPath == "" {
				t.Error("RealPath is empty")
			}
		})
	}
}

func TestResolver_Traversal(t *testing.T) {
	dir := setupTestDir(t)
	r := NewResolver(dir, SymlinkDeny, nil)

	// These should be rejected or resolve to files outside root.
	bad := []string{
		"/../../../etc/passwd",
		"/../etc/passwd",
		"/public/../../etc/passwd",
	}
	for _, path := range bad {
		t.Run(path, func(t *testing.T) {
			rf, err := r.Resolve(path)
			if err == nil {
				rf.Close()
				t.Error("expected traversal error")
			}
		})
	}
}

func TestResolver_SymlinkDeny(t *testing.T) {
	dir := setupTestDir(t)

	// Create a symlink inside root pointing outside.
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "outside.txt"), "escape!")

	symlinkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(filepath.Join(outside, "outside.txt"), symlinkPath); err != nil {
		t.Fatal(err)
	}

	r := NewResolver(dir, SymlinkDeny, nil)
	rf, err := r.Resolve("/link.txt")
	if err != ErrSymlinkDenied {
		if rf != nil {
			rf.Close()
		}
		t.Errorf("got %v, want ErrSymlinkDenied", err)
	}
}

func TestResolver_SymlinkWithinRoot(t *testing.T) {
	dir := setupTestDir(t)

	// Create a symlink inside root pointing to another file inside root.
	symlinkPath := filepath.Join(dir, "link.php")
	if err := os.Symlink("index.php", symlinkPath); err != nil {
		t.Fatal(err)
	}

	r := NewResolver(dir, SymlinkWithinRoot, nil)
	rf, err := r.Resolve("/link.php")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rf.Close()
}

func TestResolver_SymlinkEscape(t *testing.T) {
	dir := setupTestDir(t)

	// Create a symlink inside root pointing outside.
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "escape.txt"), "pwned")

	symlinkPath := filepath.Join(dir, "escape.txt")
	if err := os.Symlink(filepath.Join(outside, "escape.txt"), symlinkPath); err != nil {
		t.Fatal(err)
	}

	r := NewResolver(dir, SymlinkWithinRoot, nil)
	rf, err := r.Resolve("/escape.txt")
	if err != ErrSymlinkEscape {
		if rf != nil {
			rf.Close()
		}
		t.Errorf("got %v, want ErrSymlinkEscape", err)
	}
}

func TestResolver_ResolveInfo(t *testing.T) {
	dir := setupTestDir(t)
	r := NewResolver(dir, SymlinkDeny, nil)

	info, err := r.ResolveInfo("/index.php")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name() != "index.php" {
		t.Errorf("Name = %q, want %q", info.Name(), "index.php")
	}
}

func TestResolver_ReadAll(t *testing.T) {
	dir := setupTestDir(t)
	r := NewResolver(dir, SymlinkDeny, nil)

	rf, err := r.Resolve("/index.php")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rf.Close()

	data, err := rf.ReadAll(1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "<?php echo 'hello';" {
		t.Errorf("got %q", string(data))
	}
}

func TestResolver_ReadAllTooLarge(t *testing.T) {
	dir := setupTestDir(t)
	r := NewResolver(dir, SymlinkDeny, nil)

	rf, err := r.Resolve("/style.css")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rf.Close()

	// Try to read with a tiny max size.
	_, err = rf.ReadAll(1)
	if err == nil {
		t.Error("expected error for oversized file")
	}
}
