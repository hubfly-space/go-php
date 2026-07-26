package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockFileSaveLoadVerify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway.lock")

	lf := NewLockFile("8.3.6", "php:8.3.6:linux:amd64:fpm:abc", "hash123", "web-standard", "0.1.0", []LockExtension{
		{Name: "redis", Version: "6.0.2", SHA256: "def456", Enabled: true},
		{Name: "mbstring", Version: "8.3.0", SHA256: "ghi789", Enabled: true},
	})

	// Save.
	if err := SaveLockFile(path, lf); err != nil {
		t.Fatal(err)
	}

	// Load.
	loaded, err := LoadLockFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Verify.
	if err := loaded.Verify(); err != nil {
		t.Errorf("verify failed: %v", err)
	}

	// Check fields.
	if loaded.PHPVersion != "8.3.6" {
		t.Errorf("php_version = %q, want %q", loaded.PHPVersion, "8.3.6")
	}
	if loaded.RuntimeID != "php:8.3.6:linux:amd64:fpm:abc" {
		t.Errorf("runtime_id = %q", loaded.RuntimeID)
	}
	if loaded.Checksum == "" {
		t.Error("checksum should not be empty")
	}
	if len(loaded.Extensions) != 2 {
		t.Errorf("extensions = %d, want 2", len(loaded.Extensions))
	}
}

func TestLockFileTamperDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway.lock")

	lf := NewLockFile("8.3.6", "runtime", "hash", "", "0.1.0", nil)
	SaveLockFile(path, lf)

	// Tamper: change php_version to a different value.
	data := []byte(`{"schema":"gateway-lock/v1","php_version":"9.9.9","runtime_id":"x","manifest_hash":"x","extensions":null,"generated_at":"2025-01-01T00:00:00Z","gateway_version":"0.1.0","checksum":"` + `deadbeef"}`)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadLockFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Verify(); err == nil {
		t.Error("expected verify to fail after tampering")
	}
}

func TestLockFileValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.lock")

	// Missing schema.
	os.WriteFile(path, []byte(`{"php_version":"8.3.6"}`), 0644)
	_, err := LoadLockFile(path)
	if err == nil {
		t.Error("expected error for missing schema")
	}
}

func TestLockFileNonexistent(t *testing.T) {
	_, err := LoadLockFile("/nonexistent/gateway.lock")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestNewLockFile(t *testing.T) {
	exts := []LockExtension{
		{Name: "redis", Version: "6.0.2"},
	}

	lf := NewLockFile("8.3.6", "runtime-id", "hash", "web-standard", "0.1.0", exts)

	if lf.Schema != "gateway-lock/v1" {
		t.Errorf("schema = %q, want %q", lf.Schema, "gateway-lock/v1")
	}
	if lf.PHPVersion != "8.3.6" {
		t.Errorf("php_version = %q", lf.PHPVersion)
	}
	if lf.Profile != "web-standard" {
		t.Errorf("profile = %q", lf.Profile)
	}
}
