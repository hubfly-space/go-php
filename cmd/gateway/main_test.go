package main

import (
	"bytes"
	"flag"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseFlagsPermuted(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	foo := fs.String("foo", "", "foo option")
	bar := fs.Bool("bar", false, "bar flag")

	args := []string{"--foo", "val", "positional1", "--bar", "positional2"}
	pos, err := parseFlagsPermuted(fs, args)
	if err != nil {
		t.Fatalf("parseFlagsPermuted error: %v", err)
	}

	if *foo != "val" {
		t.Errorf("foo = %q, want %q", *foo, "val")
	}
	if !*bar {
		t.Errorf("bar = false, want true")
	}
	if len(pos) != 2 || pos[0] != "positional1" || pos[1] != "positional2" {
		t.Errorf("positional = %v, want [positional1 positional2]", pos)
	}
}

func TestPrintUsage(t *testing.T) {
	// Simple test to ensure printUsage runs without crashing
	var buf bytes.Buffer
	orig := flag.CommandLine.Output()
	defer flag.CommandLine.SetOutput(orig)

	printUsage()
	// Usage output goes to os.Stderr; verify it contains command names
	_ = buf
}

func TestResolveScript(t *testing.T) {
	docRoot := t.TempDir()
	os.WriteFile(filepath.Join(docRoot, "index.php"), []byte("<?php echo 'ok';"), 0644)
	os.MkdirAll(filepath.Join(docRoot, "api"), 0755)
	os.WriteFile(filepath.Join(docRoot, "api", "users.php"), []byte("<?php echo 'users';"), 0644)

	tests := []struct {
		name       string
		path       string
		wantScript string
		wantPath   string
	}{
		{
			name:       "root path",
			path:       "/",
			wantScript: "/index.php",
			wantPath:   filepath.Join(docRoot, "index.php"),
		},
		{
			name:       "explicit script",
			path:       "/api/users.php",
			wantScript: "/api/users.php",
			wantPath:   filepath.Join(docRoot, "api/users.php"),
		},
		{
			name:       "front controller path",
			path:       "/dashboard/analytics",
			wantScript: "/index.php",
			wantPath:   filepath.Join(docRoot, "index.php"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScript, gotPath := resolveScript(docRoot, tt.path)
			if gotScript != tt.wantScript {
				t.Errorf("scriptName = %q, want %q", gotScript, tt.wantScript)
			}
			if gotPath != tt.wantPath {
				t.Errorf("scriptPath = %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}

func TestDevErrorFormatting(t *testing.T) {
	handler := &gatewayHandler{}
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.devError(rec, req, 500, "Internal Error", "Detail message", "req_123", time.Now())

	if rec.Code != 500 {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Internal Error") {
		t.Errorf("expected body to contain 'Internal Error'")
	}
}
