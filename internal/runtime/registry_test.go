package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryExtensionManager(t *testing.T) {
	root := t.TempDir()
	r := NewRegistry(root)

	// Init creates the directory structure
	if err := r.Init(); err != nil {
		t.Fatal(err)
	}

	// Install with extensions.
	manifest := &Manifest{
		Version:  "8.3.0",
		Platform: "linux",
		Arch:     "amd64",
		Flavor:   "standard",
	}

	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "php-fpm"), []byte("binary"), 0755)
	os.MkdirAll(filepath.Join(srcDir, "lib", "php", "extensions"), 0755)

	extensions := []Extension{
		{Name: "curl", Version: "1.0"},
		{Name: "json", Version: "1.0"},
	}

	rt, err := r.Install(manifest, srcDir, extensions)
	if err != nil {
		t.Fatal(err)
	}
	if rt == nil {
		t.Fatal("expected runtime")
	}

	em := r.ExtensionManager(rt.ID)
	if em == nil {
		t.Fatal("expected ExtensionManager")
	}
	if em.RuntimeDir != r.RuntimeDir(rt.ID) {
		t.Errorf("RuntimeDir mismatch: %s vs %s", em.RuntimeDir, r.RuntimeDir(rt.ID))
	}
}

func TestRegistryListExtensionsNoManifest(t *testing.T) {
	root := t.TempDir()
	r := NewRegistry(root)
	if err := r.Init(); err != nil {
		t.Fatal(err)
	}

	exts, err := r.ListExtensions("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(exts) != 0 {
		t.Errorf("expected 0 extensions, got %d", len(exts))
	}
}

func TestRegistryVerifyExtensions(t *testing.T) {
	root := t.TempDir()
	r := NewRegistry(root)
	if err := r.Init(); err != nil {
		t.Fatal(err)
	}

	manifest := &Manifest{
		Version:  "8.3.0",
		Platform: "linux",
		Arch:     "amd64",
		Flavor:   "standard",
	}

	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "php-fpm"), []byte("binary"), 0755)
	extDir := filepath.Join(srcDir, "lib", "php", "extensions")
	os.MkdirAll(extDir, 0755)
	os.WriteFile(filepath.Join(extDir, "curl.so"), []byte("dummy"), 0644)
	os.WriteFile(filepath.Join(extDir, "json.so"), []byte("dummy"), 0644)

	extensions := []Extension{
		{Name: "curl", Version: "1.0"},
		{Name: "json", Version: "1.0"},
	}

	rt, err := r.Install(manifest, srcDir, extensions)
	if err != nil {
		t.Fatal(err)
	}

	// Verify extensions that exist.
	if err := r.VerifyExtensions(rt.ID, []string{"curl", "json"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify missing extensions.
	if err := r.VerifyExtensions(rt.ID, []string{"nonexistent"}); err == nil {
		t.Error("expected error for missing extension")
	}
}

func TestRegistryInstallStoresExtensions(t *testing.T) {
	root := t.TempDir()
	r := NewRegistry(root)
	if err := r.Init(); err != nil {
		t.Fatal(err)
	}

	manifest := &Manifest{
		Version:  "8.2.0",
		Platform: "linux",
		Arch:     "amd64",
		Flavor:   "standard",
	}

	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "php-fpm"), []byte("binary"), 0755)

	extensions := []Extension{
		{Name: "pdo", Version: "1.0"},
		{Name: "mbstring", Version: "1.0"},
	}

	rt, err := r.Install(manifest, srcDir, extensions)
	if err != nil {
		t.Fatal(err)
	}

	if len(rt.Extensions) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(rt.Extensions))
	}

	hasPDO := false
	hasMBString := false
	for _, e := range rt.Extensions {
		switch e.Name {
		case "pdo":
			hasPDO = true
		case "mbstring":
			hasMBString = true
		}
	}
	if !hasPDO {
		t.Error("expected pdo extension")
	}
	if !hasMBString {
		t.Error("expected mbstring extension")
	}
}
