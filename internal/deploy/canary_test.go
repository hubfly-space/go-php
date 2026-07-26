package deploy

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestCanarySwitcher_StartAndStatus(t *testing.T) {
	dir := t.TempDir()
	releaseMgr := NewReleaseManager(dir)
	hookRunner := NewHookRunner(HookConfig{})
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	switcher := NewCanarySwitcher(releaseMgr, hookRunner, logger)

	// Create two releases.
	rel1, _ := releaseMgr.Create("v1.0.0", "php-8.3", t.TempDir(), nil)
	rel2, _ := releaseMgr.Create("v1.1.0", "php-8.3", t.TempDir(), nil)

	err := switcher.StartCanary(context.Background(), rel1, rel2, 10)
	if err != nil {
		t.Fatal(err)
	}

	if switcher.CurrentWeight() != 10 {
		t.Errorf("expected weight 10, got %d", switcher.CurrentWeight())
	}

	state := switcher.CanaryStateView()
	if state == nil {
		t.Fatal("expected canary state")
	}
	if state.Status != "active" {
		t.Errorf("expected active status, got %s", state.Status)
	}
}

func TestCanarySwitcher_IncreaseWeight(t *testing.T) {
	dir := t.TempDir()
	releaseMgr := NewReleaseManager(dir)
	hookRunner := NewHookRunner(HookConfig{})
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	switcher := NewCanarySwitcher(releaseMgr, hookRunner, logger)

	rel1, _ := releaseMgr.Create("v1.0.0", "php-8.3", t.TempDir(), nil)
	rel2, _ := releaseMgr.Create("v1.1.0", "php-8.3", t.TempDir(), nil)

	switcher.StartCanary(context.Background(), rel1, rel2, 10)
	switcher.IncreaseWeight(20)

	if switcher.CurrentWeight() != 30 {
		t.Errorf("expected weight 30, got %d", switcher.CurrentWeight())
	}

	// Cap at 100.
	switcher.IncreaseWeight(200)
	if switcher.CurrentWeight() != 100 {
		t.Errorf("expected weight capped at 100, got %d", switcher.CurrentWeight())
	}
}

func TestCanarySwitcher_Rollback(t *testing.T) {
	dir := t.TempDir()
	releaseMgr := NewReleaseManager(dir)
	hookRunner := NewHookRunner(HookConfig{})
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	switcher := NewCanarySwitcher(releaseMgr, hookRunner, logger)

	rel1, _ := releaseMgr.Create("v1.0.0", "php-8.3", t.TempDir(), nil)
	rel2, _ := releaseMgr.Create("v1.1.0", "php-8.3", t.TempDir(), nil)

	switcher.StartCanary(context.Background(), rel1, rel2, 10)

	err := switcher.Rollback(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if switcher.CurrentWeight() != 0 {
		t.Errorf("expected weight 0 after rollback, got %d", switcher.CurrentWeight())
	}
}

func TestCanarySwitcher_NoCanary(t *testing.T) {
	dir := t.TempDir()
	releaseMgr := NewReleaseManager(dir)
	hookRunner := NewHookRunner(HookConfig{})
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	switcher := NewCanarySwitcher(releaseMgr, hookRunner, logger)

	err := switcher.Rollback(context.Background())
	if err == nil {
		t.Error("expected error when no canary")
	}

	if switcher.CurrentWeight() != 0 {
		t.Error("expected weight 0 with no canary")
	}
}

func TestCanarySwitcher_ShouldRouteToCanary(t *testing.T) {
	dir := t.TempDir()
	releaseMgr := NewReleaseManager(dir)
	hookRunner := NewHookRunner(HookConfig{})
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	switcher := NewCanarySwitcher(releaseMgr, hookRunner, logger)

	// No canary — should not route.
	if switcher.ShouldRouteToCanary() {
		t.Error("expected false with no canary")
	}
}
