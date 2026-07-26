package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseManagerCreateAndList(t *testing.T) {
	dir := t.TempDir()
	mgr := NewReleaseManager(dir)
	mgr.Init()

	// Create source.
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.php"), []byte("<?php echo 'hi';"), 0644)

	rel, err := mgr.Create("1.0.0", "php:8.3.6:linux:amd64:fpm:abc", src, nil)
	if err != nil {
		t.Fatal(err)
	}

	if rel.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", rel.Version, "1.0.0")
	}
	if rel.State != StateCreated {
		t.Errorf("state = %q, want %q", rel.State, StateCreated)
	}

	releases := mgr.List()
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
}

func TestReleaseManagerActivateDeactivate(t *testing.T) {
	dir := t.TempDir()
	mgr := NewReleaseManager(dir)
	mgr.Init()

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.php"), []byte("hi"), 0644)

	rel1, _ := mgr.Create("1.0.0", "runtime", src, nil)
	rel2, _ := mgr.Create("1.1.0", "runtime", src, nil)

	// Activate first.
	if err := mgr.Activate(rel1.ID); err != nil {
		t.Fatal(err)
	}
	active := mgr.Active()
	if active == nil || active.ID != rel1.ID {
		t.Error("expected rel1 active")
	}

	// Check symlink.
	if _, err := os.Stat(filepath.Join(dir, "active")); err != nil {
		t.Error("active symlink should exist")
	}

	// Activate second (should deactivate first).
	if err := mgr.Activate(rel2.ID); err != nil {
		t.Fatal(err)
	}
	active = mgr.Active()
	if active == nil || active.ID != rel2.ID {
		t.Error("expected rel2 active")
	}

	// Deactivate.
	if err := mgr.Deactivate(); err != nil {
		t.Fatal(err)
	}
	if mgr.Active() != nil {
		t.Error("expected no active release")
	}
}

func TestReleaseManagerRollback(t *testing.T) {
	dir := t.TempDir()
	mgr := NewReleaseManager(dir)
	mgr.Init()

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.php"), []byte("hi"), 0644)

	rel1, _ := mgr.Create("1.0.0", "runtime", src, nil)
	rel2, _ := mgr.Create("1.1.0", "runtime", src, nil)

	mgr.Activate(rel1.ID)
	mgr.Activate(rel2.ID)

	rolled, err := mgr.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if rolled.ID != rel1.ID {
		t.Errorf("rolled back to %q, want %q", rolled.ID, rel1.ID)
	}

	active := mgr.Active()
	if active == nil || active.ID != rel1.ID {
		t.Error("expected rel1 active after rollback")
	}
}

func TestReleaseManagerRollbackNoPrevious(t *testing.T) {
	dir := t.TempDir()
	mgr := NewReleaseManager(dir)
	mgr.Init()

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.php"), []byte("hi"), 0644)

	rel, _ := mgr.Create("1.0.0", "runtime", src, nil)
	mgr.Activate(rel.ID)

	_, err := mgr.Rollback()
	if err == nil {
		t.Error("expected error when no previous release")
	}
}

func TestReleaseManagerGet(t *testing.T) {
	dir := t.TempDir()
	mgr := NewReleaseManager(dir)
	mgr.Init()

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.php"), []byte("hi"), 0644)

	rel, _ := mgr.Create("1.0.0", "runtime", src, nil)

	got, err := mgr.Get(rel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.0.0" {
		t.Errorf("version = %q", got.Version)
	}

	_, err = mgr.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent release")
	}
}

func TestReleaseManagerArchive(t *testing.T) {
	dir := t.TempDir()
	mgr := NewReleaseManager(dir)
	mgr.Init()

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.php"), []byte("hi"), 0644)

	// Create releases and stop most of them.
	for i := 0; i < 5; i++ {
		rel, _ := mgr.Create("1.0.0", "runtime", src, nil)
		if i < 3 {
			mgr.Activate(rel.ID)
			mgr.Deactivate()
		}
	}

	releases := mgr.List()
	stoppedCount := 0
	for _, r := range releases {
		if r.State == StateStopped {
			stoppedCount++
		}
	}

	// Archive keeps last 1 stopped release.
	removed, err := mgr.Archive(1)
	if err != nil {
		t.Fatal(err)
	}
	if removed != stoppedCount-1 {
		t.Errorf("removed = %d, want %d (stopped=%d)", removed, stoppedCount-1, stoppedCount)
	}
}

func TestReleaseManagerMetadata(t *testing.T) {
	dir := t.TempDir()
	mgr := NewReleaseManager(dir)
	mgr.Init()

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.php"), []byte("hi"), 0644)

	rel, err := mgr.Create("1.0.0", "runtime", src, map[string]string{
		"author": "test",
		"reason": "bugfix",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := mgr.Get(rel.ID)
	if got.Metadata["author"] != "test" {
		t.Errorf("metadata[author] = %q", got.Metadata["author"])
	}
}

func TestDeploySwitcherDeploy(t *testing.T) {
	dir := t.TempDir()
	mgr := NewReleaseManager(dir)
	mgr.Init()

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.php"), []byte("hi"), 0644)

	hooks := NewHookRunner(HookConfig{})
	switcher := NewSwitcher(mgr, hooks, nil)

	result, err := switcher.Deploy(context.Background(), "1.0.0", "runtime", src, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Success {
		t.Errorf("deploy not successful: %s", result.Error)
	}
	if result.Release == nil {
		t.Fatal("expected release in result")
	}
	if len(result.Steps) < 3 {
		t.Errorf("expected at least 3 steps, got %d", len(result.Steps))
	}

	active := mgr.Active()
	if active == nil || active.ID != result.Release.ID {
		t.Error("expected deployed release to be active")
	}
}

func TestDeploySwitcherRollback(t *testing.T) {
	dir := t.TempDir()
	mgr := NewReleaseManager(dir)
	mgr.Init()

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.php"), []byte("hi"), 0644)

	hooks := NewHookRunner(HookConfig{})
	switcher := NewSwitcher(mgr, hooks, nil)

	r1, _ := switcher.Deploy(context.Background(), "1.0.0", "runtime", src, nil)
	r2, _ := switcher.Deploy(context.Background(), "1.1.0", "runtime", src, nil)

	result, err := switcher.Rollback(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("rollback not successful")
	}
	if result.Release.ID != r1.Release.ID {
		t.Errorf("rolled back to %q, want %q", result.Release.ID, r1.Release.ID)
	}
	_ = r2
}
