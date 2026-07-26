package supervisor

import (
	"os/exec"
	"testing"
)

func TestIsolator_ApplyProcessIsolation(t *testing.T) {
	config := &IsolationConfig{
		Enabled:    true,
		Mode:       "process",
		NoNewPrivs: true,
	}

	iso := NewIsolator(config)

	cmd := exec.Command("echo", "test")
	if err := iso.ApplyIsolation(cmd); err != nil {
		t.Fatal(err)
	}

	if cmd.SysProcAttr == nil {
		t.Error("expected SysProcAttr to be set")
	}
}

func TestIsolator_Disabled(t *testing.T) {
	iso := NewIsolator(nil)

	cmd := exec.Command("echo", "test")
	if err := iso.ApplyIsolation(cmd); err != nil {
		t.Fatal(err)
	}

	// SysProcAttr should NOT be set when disabled.
	if cmd.SysProcAttr != nil {
		t.Error("expected SysProcAttr to be nil when disabled")
	}
}

func TestIsolator_Cleanup(t *testing.T) {
	config := &IsolationConfig{
		Enabled:    true,
		Mode:       "cgroup",
		CgroupPath: "test-gateway-cleanup",
	}

	iso := NewIsolator(config)
	iso.Cleanup() // Should not panic.
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"256M", 256 * 1024 * 1024},
		{"1G", 1024 * 1024 * 1024},
		{"512K", 512 * 1024},
		{"1024", 1024},
		{"", 0},
		{"100", 100},
	}

	for _, tt := range tests {
		result := parseBytes(tt.input)
		if result != tt.expected {
			t.Errorf("parseBytes(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestDefaultIsolationConfig(t *testing.T) {
	config := DefaultIsolationConfig()

	if !config.NoNewPrivs {
		t.Error("expected NoNewPrivs to be true")
	}
	if config.PIDLimit != 64 {
		t.Errorf("expected PID limit 64, got %d", config.PIDLimit)
	}
}

func TestNewIsolator_NilConfig(t *testing.T) {
	iso := NewIsolator(nil)
	if iso.config == nil {
		t.Error("expected default config when nil passed")
	}
}
