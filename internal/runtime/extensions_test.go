package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtensionManagerEnableWithType(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	if err := mgr.EnableWithType("xdebug", "zend_extension"); err != nil {
		t.Fatal(err)
	}
	if !mgr.IsEnabled("xdebug") {
		t.Error("xdebug should be enabled")
	}

	data, err := os.ReadFile(filepath.Join(dir, "conf.d", "20-xdebug.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "zend_extension=xdebug.so\n" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestExtensionManagerBulkEnable(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	exts := []string{"curl", "json", "pdo"}
	if err := mgr.BulkEnable(exts); err != nil {
		t.Fatal(err)
	}

	for _, name := range exts {
		if !mgr.IsEnabled(name) {
			t.Errorf("%s should be enabled", name)
		}
	}
}

func TestExtensionManagerBulkDisable(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	if err := mgr.BulkEnable([]string{"curl", "json"}); err != nil {
		t.Fatal(err)
	}

	if err := mgr.BulkDisable([]string{"curl"}); err != nil {
		t.Fatal(err)
	}

	if mgr.IsEnabled("curl") {
		t.Error("curl should be disabled")
	}
	if !mgr.IsEnabled("json") {
		t.Error("json should still be enabled")
	}
}

func TestExtensionManagerGetEnabled(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	enabled, err := mgr.GetEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 0 {
		t.Errorf("expected no enabled extensions, got %v", enabled)
	}

	if err := mgr.Enable("pdo"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Enable("curl"); err != nil {
		t.Fatal(err)
	}

	enabled, err = mgr.GetEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 2 {
		t.Fatalf("expected 2 enabled, got %d", len(enabled))
	}
	if enabled[0] != "curl" || enabled[1] != "pdo" {
		t.Errorf("expected [curl pdo], got %v", enabled)
	}
}

func TestExtensionManagerValidateExists(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	if mgr.ValidateExists("nonexistent_extension_xyz") {
		t.Error("should not find nonexistent extension")
	}

	extDir := filepath.Join(dir, "lib", "php", "extensions")
	os.MkdirAll(extDir, 0755)
	os.WriteFile(filepath.Join(extDir, "custom.so"), []byte("dummy"), 0644)

	if !mgr.ValidateExists("custom") {
		t.Error("should find custom extension")
	}
}

func TestExtensionManagerApplyExtensionsEnabled(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	// Enable some extensions manually first.
	if err := mgr.BulkEnable([]string{"curl", "json", "pdo"}); err != nil {
		t.Fatal(err)
	}

	// Apply a subset.
	err := mgr.ApplyExtensions([]configResolvedExtension{
		{Name: "pdo", Type: "extension"},
		{Name: "curl", Type: "extension"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !mgr.IsEnabled("curl") {
		t.Error("curl should be enabled")
	}
	if !mgr.IsEnabled("pdo") {
		t.Error("pdo should be enabled")
	}
	if mgr.IsEnabled("json") {
		t.Error("json should have been disabled")
	}
}

func TestExtensionManagerApplyExtensionsChangesType(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	if err := mgr.Enable("xdebug"); err != nil {
		t.Fatal(err)
	}

	err := mgr.ApplyExtensions([]configResolvedExtension{
		{Name: "xdebug", Type: "zend_extension"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "conf.d", "20-xdebug.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "zend_extension=xdebug.so\n" {
		t.Errorf("expected zend_extension, got: %q", string(data))
	}
}

func TestExtensionManagerApplyExtensionsEmpty(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	if err := mgr.BulkEnable([]string{"curl", "json"}); err != nil {
		t.Fatal(err)
	}

	if err := mgr.ApplyExtensions(nil); err != nil {
		t.Fatal(err)
	}

	enabled, _ := mgr.GetEnabled()
	if len(enabled) != 0 {
		t.Errorf("expected all disabled, got %v", enabled)
	}
}

func TestExtensionManagerListInstalledNoManifest(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	exts, err := mgr.ListInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(exts) != 0 {
		t.Errorf("expected no extensions, got %v", exts)
	}
}

func TestExtensionManagerDoubleDisable(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	if err := mgr.Enable("redis"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Disable("redis"); err != nil {
		t.Fatal(err)
	}
	// Second disable should be fine.
	if err := mgr.Disable("redis"); err != nil {
		t.Fatal(err)
	}
	if mgr.IsEnabled("redis") {
		t.Error("redis should be disabled")
	}
}
