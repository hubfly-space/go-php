package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateConfigPhpIni(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		PHPBinary:   "/usr/sbin/php-fpm",
		SocketPath:  filepath.Join(dir, "php-fpm.sock"),
		PIDFile:     filepath.Join(dir, "php-fpm.pid"),
		ErrorLog:    filepath.Join(dir, "error.log"),
		RuntimeDir:  dir,
		PhpIni: []IniSetting{
			{Name: "memory_limit", Value: "256M"},
			{Name: "upload_max_filesize", Value: "64M"},
		},
	}
	s := New(cfg)
	path, err := s.generateConfig()
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "php_admin_value[memory_limit] = 256M") {
		t.Error("expected memory_limit directive")
	}
	if !strings.Contains(content, "php_admin_value[upload_max_filesize] = 64M") {
		t.Error("expected upload_max_filesize directive")
	}
}

func TestGenerateConfigExtensionsComment(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		PHPBinary:  "/usr/sbin/php-fpm",
		SocketPath: filepath.Join(dir, "php-fpm.sock"),
		PIDFile:    filepath.Join(dir, "php-fpm.pid"),
		ErrorLog:   filepath.Join(dir, "error.log"),
		RuntimeDir: dir,
		Extensions: []Extension{
			{Name: "curl", Type: "extension"},
			{Name: "xdebug", Type: "zend_extension"},
		},
	}
	s := New(cfg)
	path, err := s.generateConfig()
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "php_admin_value[extension] = curl.so") {
		t.Error("expected curl.so extension directive")
	}
	if !strings.Contains(content, "php_admin_value[zend_extension] = xdebug.so") {
		t.Error("expected xdebug.so zend_extension directive")
	}
}

func TestGenerateConfigEmptyExtensions(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		PHPBinary:  "/usr/sbin/php-fpm",
		SocketPath: filepath.Join(dir, "php-fpm.sock"),
		PIDFile:    filepath.Join(dir, "php-fpm.pid"),
		ErrorLog:   filepath.Join(dir, "error.log"),
		RuntimeDir: dir,
		Extensions: nil,
	}
	s := New(cfg)
	path, err := s.generateConfig()
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if strings.Contains(content, "Extensions loaded from resolved config") {
		t.Error("should not have extension section when no extensions")
	}
}

func TestGenerateConfigPhpIniDeduplicates(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		PHPBinary:  "/usr/sbin/php-fpm",
		SocketPath: filepath.Join(dir, "php-fpm.sock"),
		PIDFile:    filepath.Join(dir, "php-fpm.pid"),
		ErrorLog:   filepath.Join(dir, "error.log"),
		RuntimeDir: dir,
		PhpIni: []IniSetting{
			{Name: "memory_limit", Value: "256M"},
			{Name: "memory_limit", Value: "512M"},
		},
	}
	s := New(cfg)
	path, err := s.generateConfig()
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	count := strings.Count(content, "php_admin_value[memory_limit]")
	if count != 1 {
		t.Errorf("expected 1 memory_limit directive, got %d", count)
	}
}

func TestGenerateConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		PHPBinary:  "/usr/sbin/php-fpm",
		SocketPath: filepath.Join(dir, "php-fpm.sock"),
		PIDFile:    filepath.Join(dir, "php-fpm.pid"),
		ErrorLog:   filepath.Join(dir, "error.log"),
	}
	s := New(cfg)
	path, err := s.generateConfig()
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "pm.max_children = 20") {
		t.Error("expected default max_children = 20")
	}
	if !strings.Contains(content, "pm.start_servers = 2") {
		t.Error("expected default start_servers = 2")
	}
}

func TestGenerateConfigExplicitValues(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		PHPBinary:      "/usr/sbin/php-fpm",
		SocketPath:     filepath.Join(dir, "php-fpm.sock"),
		PIDFile:        filepath.Join(dir, "php-fpm.pid"),
		ErrorLog:       filepath.Join(dir, "error.log"),
		MaxChildren:    10,
		StartServers:   4,
		MinSpare:       2,
		MaxSpare:       8,
		MaxRequests:    1000,
		RequestTimeout: 60 * time.Second,
	}
	s := New(cfg)
	path, err := s.generateConfig()
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "pm.max_children = 10") {
		t.Error("expected max_children = 10")
	}
	if !strings.Contains(content, "pm.start_servers = 4") {
		t.Error("expected start_servers = 4")
	}
	if !strings.Contains(content, "pm.max_requests = 1000") {
		t.Error("expected max_requests = 1000")
	}
	if !strings.Contains(content, "request_terminate_timeout = 60s") {
		t.Error("expected request_terminate_timeout = 60s")
	}
}
