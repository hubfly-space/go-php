package runtime

import (
	"testing"
)

func TestProvisioner_PackageNameAndDetect(t *testing.T) {
	p := NewProvisioner("php-fpm8.3")

	if p.phpVersion != "8.3" {
		t.Errorf("phpVersion = %q, want 8.3", p.phpVersion)
	}

	pkgName := p.PackageName("mysqli")
	if pkgName != "php8.3-mysql" {
		t.Errorf("PackageName(mysqli) = %q, want php8.3-mysql", pkgName)
	}

	pkgCurl := p.PackageName("curl")
	if pkgCurl != "php8.3-curl" {
		t.Errorf("PackageName(curl) = %q, want php8.3-curl", pkgCurl)
	}
}

func TestProvisioner_MissingExtensions(t *testing.T) {
	p := NewProvisioner("php-fpm8.3")

	// Since extensionDir is empty or default, missing extensions should return all requested
	p.extensionDir = t.TempDir()

	missing := p.MissingExtensions([]string{"mysqli", "curl"})
	if len(missing) != 2 {
		t.Errorf("expected 2 missing extensions, got %d", len(missing))
	}
}
