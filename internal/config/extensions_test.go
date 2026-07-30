package config

import (
	"testing"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestResolveExtensionsWithProfile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.ExtensionProfile = "web-standard"
	cfg.PHP.Extensions = nil

	resolved, ini, err := ResolveExtensions(&cfg.PHP)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) == 0 {
		t.Error("expected resolved extensions")
	}
	if ini != nil {
		t.Errorf("expected nil ini when no php_ini configured, got %v", ini)
	}
}

func TestResolveExtensionsWithIndividualEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.ExtensionProfile = ""
	cfg.PHP.Extensions = []ExtensionConfig{
		{Name: "curl", Enabled: boolPtr(true)},
		{Name: "json", Enabled: boolPtr(true)},
		{Name: "redis", Enabled: boolPtr(false)},
	}

	resolved, _, err := ResolveExtensions(&cfg.PHP)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(resolved))
	}
	if resolved[0].Name != "curl" {
		t.Errorf("expected curl first, got %s", resolved[0].Name)
	}
}

func TestResolveExtensionsWithNilEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.ExtensionProfile = ""
	cfg.PHP.Extensions = []ExtensionConfig{
		{Name: "pdo"},
		{Name: "gd", Enabled: boolPtr(true)},
		{Name: "xdebug", Enabled: boolPtr(false)},
	}

	resolved, _, err := ResolveExtensions(&cfg.PHP)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(resolved))
	}
}

func TestResolveExtensionsWithType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.ExtensionProfile = ""
	cfg.PHP.Extensions = []ExtensionConfig{
		{Name: "xdebug", Type: "zend_extension", Enabled: boolPtr(true)},
		{Name: "opcache", Enabled: boolPtr(true)},
	}

	resolved, _, err := ResolveExtensions(&cfg.PHP)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(resolved))
	}
	if resolved[0].Name == "xdebug" && resolved[0].Type != "zend_extension" {
		t.Errorf("expected zend_extension type, got %s", resolved[0].Type)
	}
}

func TestResolveExtensionsEmpty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.ExtensionProfile = ""
	cfg.PHP.Extensions = nil

	resolved, ini, err := ResolveExtensions(&cfg.PHP)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != nil {
		t.Errorf("expected nil resolved, got %v", resolved)
	}
	if ini == nil {
		t.Error("expected non-nil ini")
	}
}

func TestResolveExtensionsAllDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.ExtensionProfile = ""
	cfg.PHP.Extensions = []ExtensionConfig{
		{Name: "curl", Enabled: boolPtr(false)},
	}

	resolved, _, err := ResolveExtensions(&cfg.PHP)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 0 {
		t.Errorf("expected 0 extensions, got %d", len(resolved))
	}
}

func TestResolveExtensionsUnknownProfile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.ExtensionProfile = "nonexistent"

	_, _, err := ResolveExtensions(&cfg.PHP)
	if err == nil {
		t.Error("expected error for unknown profile")
	}
}

func TestApplyExtensionOverridesNil(t *testing.T) {
	base := []ResolvedExtension{{Name: "curl", Type: "extension"}}
	result := ApplyExtensionOverrides(base, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(result))
	}
}

func TestApplyExtensionOverridesDisable(t *testing.T) {
	base := []ResolvedExtension{
		{Name: "curl", Type: "extension"},
		{Name: "json", Type: "extension"},
		{Name: "pdo", Type: "extension"},
	}
	override := &ExtensionOverride{
		Disable: []string{"json"},
	}
	result := ApplyExtensionOverrides(base, override)
	if len(result) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(result))
	}
	for _, ext := range result {
		if ext.Name == "json" {
			t.Error("json should have been disabled")
		}
	}
}

func TestApplyExtensionOverridesEnableNew(t *testing.T) {
	base := []ResolvedExtension{
		{Name: "curl", Type: "extension"},
	}
	override := &ExtensionOverride{
		Enable: []string{"redis", "memcached"},
	}
	result := ApplyExtensionOverrides(base, override)
	if len(result) != 3 {
		t.Fatalf("expected 3 extensions, got %d", len(result))
	}
}

func TestApplyExtensionOverridesEnableExisting(t *testing.T) {
	base := []ResolvedExtension{
		{Name: "curl", Type: "extension"},
	}
	override := &ExtensionOverride{
		Enable: []string{"curl", "redis"},
	}
	result := ApplyExtensionOverrides(base, override)
	if len(result) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(result))
	}
}

func TestApplyExtensionOverridesDisableAndEnable(t *testing.T) {
	base := []ResolvedExtension{
		{Name: "curl", Type: "extension"},
		{Name: "json", Type: "extension"},
	}
	override := &ExtensionOverride{
		Disable: []string{"curl"},
		Enable:  []string{"pdo"},
	}
	result := ApplyExtensionOverrides(base, override)
	if len(result) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(result))
	}
}

func TestValidateExtensionProfile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.ExtensionProfile = "web-standard"
	if err := Validate(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateExtensionUnknownProfile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.ExtensionProfile = "nonexistent"
	if err := Validate(cfg); err == nil {
		t.Error("expected error for unknown extension profile")
	}
}

func TestValidateExtensionDuplicateName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.Extensions = []ExtensionConfig{
		{Name: "curl"},
		{Name: "curl"},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for duplicate extension name")
	}
}

func TestValidateExtensionInvalidType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.Extensions = []ExtensionConfig{
		{Name: "foo", Type: "invalid_type"},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for invalid extension type")
	}
}

func TestValidatePhpIniSettings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PHP.PhpIni = []IniSetting{
		{Name: "memory_limit", Value: "256M"},
		{Name: "upload_max_filesize", Value: "64M"},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultConfigExtensionFields(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.PHP.ExtensionProfile != "" {
		t.Errorf("expected empty ExtensionProfile, got %q", cfg.PHP.ExtensionProfile)
	}
	if cfg.PHP.Extensions != nil {
		t.Errorf("expected nil Extensions")
	}
	if cfg.PHP.PhpIni != nil {
		t.Errorf("expected nil PhpIni")
	}
}
