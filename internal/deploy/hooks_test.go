package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHookRunnerPrePostActivate(t *testing.T) {
	dir := t.TempDir()
	mgr := NewReleaseManager(dir)
	mgr.Init()

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.php"), []byte("hi"), 0644)

	rel, _ := mgr.Create("1.0.0", "runtime", src, nil)

	// Write a script to execute.
	script := filepath.Join(dir, "hook.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho ok"), 0755)

	cfg := HookConfig{
		PreActivate: []Hook{
			{Command: script},
		},
		PostActivate: []Hook{
			{Command: script},
		},
	}

	runner := NewHookRunner(cfg)

	if err := runner.RunPreActivate(context.Background(), rel); err != nil {
		t.Fatal(err)
	}

	if err := runner.RunPostActivate(context.Background(), rel); err != nil {
		t.Fatal(err)
	}

	entries := runner.audit.Recent(10)
	if len(entries) != 2 {
		t.Errorf("expected 2 audit entries, got %d", len(entries))
	}
}

func TestHookRunnerRejectsShellMetachars(t *testing.T) {
	runner := NewHookRunner(HookConfig{})

	rel := &Release{ID: "test", Dir: t.TempDir()}

	cfg := HookConfig{
		PreActivate: []Hook{
			{Command: "echo hello; rm -rf /"},
		},
	}
	runner.config = cfg

	err := runner.RunPreActivate(context.Background(), rel)
	if err == nil {
		t.Error("expected error for shell metacharacters")
	}
}

func TestHookRunnerTimeout(t *testing.T) {
	runner := NewHookRunner(HookConfig{})

	rel := &Release{ID: "test", Dir: t.TempDir()}

	cfg := HookConfig{
		PreActivate: []Hook{
			{Command: "sleep", Args: []string{"10"}, Timeout: 100 * time.Millisecond},
		},
	}
	runner.config = cfg

	err := runner.RunPreActivate(context.Background(), rel)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestHookRunnerEmptyCommand(t *testing.T) {
	runner := NewHookRunner(HookConfig{})

	rel := &Release{ID: "test", Dir: t.TempDir()}

	cfg := HookConfig{
		PreActivate: []Hook{
			{Command: ""},
		},
	}
	runner.config = cfg

	err := runner.RunPreActivate(context.Background(), rel)
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestHookAuditLog(t *testing.T) {
	log := NewHookAuditLog()

	log.Log(HookAuditEntry{
		Type:      HookPreActivate,
		Command:   "test",
		Duration:  100 * time.Millisecond,
		ExitCode:  0,
		ReleaseID: "rel1",
	})

	entries := log.Recent(10)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Type != HookPreActivate {
		t.Errorf("type = %q, want %q", entries[0].Type, HookPreActivate)
	}
}

func TestValidateHookConfig(t *testing.T) {
	// Valid config.
	err := ValidateHookConfig(&HookConfig{
		PreActivate: []Hook{
			{Command: "echo", Args: []string{"ok"}},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Empty command.
	err = ValidateHookConfig(&HookConfig{
		PreActivate: []Hook{
			{Command: ""},
		},
	})
	if err == nil {
		t.Error("expected error for empty command")
	}

	// Shell metacharacters.
	err = ValidateHookConfig(&HookConfig{
		PostActivate: []Hook{
			{Command: "echo hello | cat"},
		},
	})
	if err == nil {
		t.Error("expected error for shell metacharacters")
	}

	// Excessive timeout.
	err = ValidateHookConfig(&HookConfig{
		PreActivate: []Hook{
			{Command: "test", Timeout: 10 * time.Minute},
		},
	})
	if err == nil {
		t.Error("expected error for excessive timeout")
	}
}
