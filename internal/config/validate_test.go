package config

import (
	"testing"
)

func TestValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := Validate(cfg); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidate_InvalidSchema(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Schema = ""

	if err := Validate(cfg); err == nil {
		t.Error("expected error for empty schema version")
	}
}

func TestValidate_InvalidSymlinkMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.SymlinkMode = "invalid_mode"

	if err := Validate(cfg); err == nil {
		t.Error("expected error for invalid symlink mode")
	}
}

func TestValidate_InvalidMaxBodySize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.MaxBodySize = "invalid_size"

	if err := Validate(cfg); err == nil {
		t.Error("expected error for invalid max body size string")
	}
}
