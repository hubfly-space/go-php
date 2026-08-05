package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifest_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	m := &Manifest{
		Version:  "8.3.0",
		Platform: "linux",
		Arch:     "amd64",
		Flavor:   "cli",
		SHA256:   "1234567890abcdef",
		Size:     1024,
	}

	if err := SaveManifest(path, m); err != nil {
		t.Fatalf("SaveManifest failed: %v", err)
	}

	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if loaded.Version != m.Version {
		t.Errorf("Version = %q, want %q", loaded.Version, m.Version)
	}
	if loaded.Platform != m.Platform {
		t.Errorf("Platform = %q, want %q", loaded.Platform, m.Platform)
	}

	found, foundPath, err := FindManifest(dir)
	if err != nil {
		t.Fatalf("FindManifest failed: %v", err)
	}
	if foundPath != path {
		t.Errorf("foundPath = %q, want %q", foundPath, path)
	}
	if found.Version != m.Version {
		t.Errorf("found.Version = %q, want %q", found.Version, m.Version)
	}
}

func TestManifest_InvalidManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")

	// Missing version
	_ = os.WriteFile(path, []byte(`{"platform":"linux","arch":"amd64"}`), 0644)
	if _, err := LoadManifest(path); err == nil {
		t.Error("expected error for missing version")
	}
}
