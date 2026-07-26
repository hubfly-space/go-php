package fpm

import (
	"os"
	"testing"
)

func TestPoolConfigGenerate(t *testing.T) {
	cfg := DefaultPoolConfig("www")
	cfg.SocketPath = "/tmp/test.sock"
	cfg.MaxChildren = 10

	content := cfg.Generate()

	if content == "" {
		t.Error("expected non-empty config")
	}
	assertContains(t, content, "[www]")
	assertContains(t, content, "listen = /tmp/test.sock")
	assertContains(t, content, "pm.max_children = 10")
	assertContains(t, content, "clear_env = yes")
	assertContains(t, content, "security.limit_extensions = .php")
}

func TestPoolConfigWritePool(t *testing.T) {
	cfg := DefaultPoolConfig("test-pool")
	cfg.SocketPath = "/tmp/test.sock"

	dir := t.TempDir()
	path, err := cfg.WritePool(dir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, string(data), "[test-pool]")
	assertContains(t, string(data), "listen = /tmp/test.sock")
}

func TestMainConfigGenerate(t *testing.T) {
	content := GenerateMainConfig("/pools", "/tmp/php-fpm.pid", "/tmp/error.log")

	assertContains(t, content, "[global]")
	assertContains(t, content, "pid = /tmp/php-fpm.pid")
	assertContains(t, content, "error_log = /tmp/error.log")
	assertContains(t, content, "include = pools/*.conf")
}

func TestMainConfigWrite(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteMainConfig(dir, "/tmp/php-fpm.pid", "/tmp/error.log")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, string(data), "[global]")
}

func TestConfigLayeringMerge(t *testing.T) {
	cl := NewConfigLayering()

	cl.AddLayer("defaults", 100, map[string]string{
		"memory_limit":    "128M",
		"error_reporting": "E_ALL",
	})

	cl.AddLayer("project", 200, map[string]string{
		"memory_limit":   "256M",
		"display_errors": "On",
	})

	cl.AddLayer("route", 300, map[string]string{
		"memory_limit": "512M",
	})

	merged := cl.Merge()
	if merged["memory_limit"] != "512M" {
		t.Errorf("memory_limit = %q, want %q (route override)", merged["memory_limit"], "512M")
	}
	if merged["error_reporting"] != "E_ALL" {
		t.Errorf("error_reporting = %q, want %q", merged["error_reporting"], "E_ALL")
	}
	if merged["display_errors"] != "On" {
		t.Errorf("display_errors = %q, want %q", merged["display_errors"], "On")
	}
}

func TestConfigLayeringGenerateINI(t *testing.T) {
	cl := NewConfigLayering()
	cl.AddLayer("dev", 100, DefaultDevDirectives())

	ini := cl.GenerateINI()
	assertContains(t, ini, "display_errors = On")
	assertContains(t, ini, "error_reporting = E_ALL")
	assertContains(t, ini, "memory_limit = 256M")
}

func TestConfigLayeringWriteINI(t *testing.T) {
	cl := NewConfigLayering()
	cl.AddLayer("dev", 100, DefaultDevDirectives())

	dir := t.TempDir()
	path, err := cl.WriteINI(dir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, string(data), "display_errors = On")
}

func TestClassifyDirective(t *testing.T) {
	tests := []struct {
		name string
		want DirectiveClassification
	}{
		{"memory_limit", ClassWarning},
		{"display_errors", ClassOwned},
		{"disable_functions", ClassRestricted},
		{"filter.default", ClassSafe},
		{"opcache.enable", ClassWarning},
		{"opcache.memory_consumption", ClassWarning},
		{"session.cookie_httponly", ClassSafe},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyDirective(tt.name)
			if got != tt.want {
				t.Errorf("ClassifyDirective(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestDefaultDevDirectives(t *testing.T) {
	d := DefaultDevDirectives()
	if d["display_errors"] != "On" {
		t.Errorf("display_errors = %q, want On", d["display_errors"])
	}
	if d["opcache.enable"] != "0" {
		t.Errorf("opcache.enable = %q, want 0", d["opcache.enable"])
	}
}

func TestDefaultProdDirectives(t *testing.T) {
	d := DefaultProdDirectives()
	if d["display_errors"] != "Off" {
		t.Errorf("display_errors = %q, want Off", d["display_errors"])
	}
	if d["opcache.enable"] != "1" {
		t.Errorf("opcache.enable = %q, want 1", d["opcache.enable"])
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !containsStr(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
