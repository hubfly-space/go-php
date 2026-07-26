package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Schema != "gateway/v1" {
		t.Errorf("schema = %q, want %q", cfg.Schema, "gateway/v1")
	}
	if cfg.Server.Addr != ":8080" {
		t.Errorf("addr = %q, want %q", cfg.Server.Addr, ":8080")
	}
}

func TestValidateSchemaRequired(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Schema = ""
	if err := Validate(cfg); err == nil {
		t.Error("expected error for empty schema")
	}
}

func TestValidateAddrRequired(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Addr = ""
	if err := Validate(cfg); err == nil {
		t.Error("expected error for empty addr")
	}
}

func TestValidateInvalidAddr(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Addr = "not-an-addr"
	if err := Validate(cfg); err == nil {
		t.Error("expected error for invalid addr")
	}
}

func TestValidateDefaults(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logging.Format = ""
	cfg.Security.SymlinkMode = ""
	if err := Validate(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("logging.format = %q, want %q", cfg.Logging.Format, "json")
	}
}

func TestLoadNonexistent(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent config")
	}
}

func TestLoadValidConfig(t *testing.T) {
	content := `
schema: gateway/v1
server:
  addr: ":9090"
  read_timeout: 10s
logging:
  format: text
`
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.Write([]byte(content))
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Addr != ":9090" {
		t.Errorf("addr = %q, want %q", cfg.Server.Addr, ":9090")
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("format = %q, want %q", cfg.Logging.Format, "text")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.Write([]byte(`{{invalid yaml`))
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadInvalidSchema(t *testing.T) {
	content := `
schema: ""
server:
  addr: ":9090"
`
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.Write([]byte(content))
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Error("expected error for empty schema")
	}
}
