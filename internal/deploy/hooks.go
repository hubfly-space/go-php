package deploy

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// HookType represents the type of deploy hook.
type HookType string

const (
	HookPreActivate    HookType = "pre_activate"
	HookPostActivate   HookType = "post_activate"
	HookPreDeactivate  HookType = "pre_deactivate"
	HookPostDeactivate HookType = "post_deactivate"
)

// Hook represents a single deploy hook.
type Hook struct {
	Type    HookType
	Command string
	Args    []string
	Timeout time.Duration
	WorkDir string
	Env     []string
}

// HookConfig defines all hooks for a deploy.
type HookConfig struct {
	PreActivate    []Hook
	PostActivate   []Hook
	PreDeactivate  []Hook
	PostDeactivate []Hook
}

// HookRunner executes deploy hooks.
type HookRunner struct {
	config  HookConfig
	audit   *HookAuditLog
	timeout time.Duration
}

// NewHookRunner creates a hook runner.
func NewHookRunner(config HookConfig) *HookRunner {
	return &HookRunner{
		config:  config,
		audit:   NewHookAuditLog(),
		timeout: 30 * time.Second,
	}
}

// RunPreActivate runs pre-activate hooks.
func (hr *HookRunner) RunPreActivate(ctx context.Context, rel *Release) error {
	return hr.runHooks(ctx, HookPreActivate, hr.config.PreActivate, rel)
}

// RunPostActivate runs post-activate hooks.
func (hr *HookRunner) RunPostActivate(ctx context.Context, rel *Release) error {
	return hr.runHooks(ctx, HookPostActivate, hr.config.PostActivate, rel)
}

// RunPreDeactivate runs pre-deactivate hooks.
func (hr *HookRunner) RunPreDeactivate(ctx context.Context, rel *Release) error {
	return hr.runHooks(ctx, HookPreDeactivate, hr.config.PreDeactivate, rel)
}

// RunPostDeactivate runs post-deactivate hooks.
func (hr *HookRunner) RunPostDeactivate(ctx context.Context, rel *Release) error {
	return hr.runHooks(ctx, HookPostDeactivate, hr.config.PostDeactivate, rel)
}

func (hr *HookRunner) runHooks(ctx context.Context, hookType HookType, hooks []Hook, rel *Release) error {
	for _, hook := range hooks {
		if err := hr.runSingleHook(ctx, hookType, hook, rel); err != nil {
			return fmt.Errorf("hook %s/%s: %w", hookType, hook.Command, err)
		}
	}
	return nil
}

func (hr *HookRunner) runSingleHook(ctx context.Context, hookType HookType, hook Hook, rel *Release) error {
	timeout := hook.Timeout
	if timeout == 0 {
		timeout = hr.timeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	workDir := hook.WorkDir
	if workDir == "" {
		workDir = rel.Dir
	}

	// Validate: reject shell strings before splitting.
	if strings.ContainsAny(hook.Command, ";|&$`") {
		return fmt.Errorf("hook command contains shell metacharacters: %q", hook.Command)
	}

	// Build environment.
	env := os.Environ()
	env = append(env, hook.Env...)
	env = append(env,
		fmt.Sprintf("GATEWAY_RELEASE_ID=%s", rel.ID),
		fmt.Sprintf("GATEWAY_RELEASE_VERSION=%s", rel.Version),
		fmt.Sprintf("GATEWAY_RELEASE_DIR=%s", rel.Dir),
		fmt.Sprintf("GATEWAY_HOOK_TYPE=%s", hookType),
	)

	// Run command.
	args := hook.Args
	if len(args) == 0 {
		parts := strings.Fields(hook.Command)
		if len(parts) == 0 {
			return fmt.Errorf("empty command")
		}
		hook.Command = parts[0]
		if len(parts) > 1 {
			args = parts[1:]
		}
	}

	cmd := exec.CommandContext(ctx, hook.Command, args...)
	cmd.Dir = workDir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// Audit log.
	hr.audit.Log(HookAuditEntry{
		Timestamp: time.Now(),
		Type:      hookType,
		Command:   hook.Command,
		Args:      args,
		Duration:  duration,
		ExitCode:  exitCode(err),
		Stdout:    truncate(stdout.String(), 4096),
		Stderr:    truncate(stderr.String(), 4096),
		ReleaseID: rel.ID,
		Error:     errStr(err),
	})

	if err != nil {
		return fmt.Errorf("command %q failed (exit %d): %s", hook.Command, exitCode(err), stderr.String())
	}

	return nil
}

// HookAuditEntry records a hook execution.
type HookAuditEntry struct {
	Timestamp time.Time     `json:"timestamp"`
	Type      HookType      `json:"type"`
	Command   string        `json:"command"`
	Args      []string      `json:"args"`
	Duration  time.Duration `json:"duration"`
	ExitCode  int           `json:"exit_code"`
	Stdout    string        `json:"stdout,omitempty"`
	Stderr    string        `json:"stderr,omitempty"`
	ReleaseID string        `json:"release_id"`
	Error     string        `json:"error,omitempty"`
}

// HookAuditLog stores hook execution history.
type HookAuditLog struct {
	entries []HookAuditEntry
	maxSize int
}

// NewHookAuditLog creates a hook audit log.
func NewHookAuditLog() *HookAuditLog {
	return &HookAuditLog{maxSize: 100}
}

// Log records a hook execution.
func (hal *HookAuditLog) Log(entry HookAuditEntry) {
	hal.entries = append(hal.entries, entry)
	if len(hal.entries) > hal.maxSize {
		hal.entries = hal.entries[len(hal.entries)-hal.maxSize:]
	}
}

// Recent returns the last n entries.
func (hal *HookAuditLog) Recent(n int) []HookAuditEntry {
	if n > len(hal.entries) {
		n = len(hal.entries)
	}
	result := make([]HookAuditEntry, n)
	copy(result, hal.entries[len(hal.entries)-n:])
	return result
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "...(truncated)"
	}
	return s
}

// ValidateHookConfig checks a hook config for safety issues.
func ValidateHookConfig(cfg *HookConfig) error {
	hooks := append(cfg.PreActivate, cfg.PostActivate...)
	hooks = append(hooks, cfg.PreDeactivate...)
	hooks = append(hooks, cfg.PostDeactivate...)

	for _, h := range hooks {
		if h.Command == "" {
			return fmt.Errorf("empty hook command")
		}
		if strings.ContainsAny(h.Command, ";|&$`") {
			return fmt.Errorf("hook command %q contains shell metacharacters", h.Command)
		}
		if h.Timeout > 5*time.Minute {
			return fmt.Errorf("hook timeout %v exceeds maximum (5m)", h.Timeout)
		}
	}

	return nil
}
