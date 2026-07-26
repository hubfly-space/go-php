package runtime

import (
	"testing"
)

func TestExtensionManager(t *testing.T) {
	dir := t.TempDir()
	mgr := NewExtensionManager(dir)

	// Enable.
	if err := mgr.Enable("redis"); err != nil {
		t.Fatal(err)
	}
	if !mgr.IsEnabled("redis") {
		t.Error("redis should be enabled")
	}

	// Disable.
	if err := mgr.Disable("redis"); err != nil {
		t.Fatal(err)
	}
	if mgr.IsEnabled("redis") {
		t.Error("redis should be disabled")
	}

	// Double disable should be ok.
	if err := mgr.Disable("redis"); err != nil {
		t.Fatal(err)
	}
}

func TestProfileByName(t *testing.T) {
	p := ProfileByName("web-standard")
	if p == nil {
		t.Fatal("expected web-standard profile")
	}
	if len(p.Extensions) == 0 {
		t.Error("expected extensions")
	}

	if ProfileByName("nonexistent") != nil {
		t.Error("expected nil for unknown profile")
	}
}

func TestProfileExtensions(t *testing.T) {
	exts, err := ProfileExtensions("wordpress")
	if err != nil {
		t.Fatal(err)
	}
	if len(exts) == 0 {
		t.Error("expected extensions")
	}

	// Should be sorted.
	for i := 1; i < len(exts); i++ {
		if exts[i] < exts[i-1] {
			t.Errorf("extensions not sorted: %s < %s", exts[i], exts[i-1])
		}
	}

	_, err = ProfileExtensions("nonexistent")
	if err == nil {
		t.Error("expected error for unknown profile")
	}
}

func TestComputeExtensionHash(t *testing.T) {
	a := ComputeExtensionHash([]string{"redis", "mbstring"})
	b := ComputeExtensionHash([]string{"mbstring", "redis"})
	if a != b {
		t.Error("hash should be order-independent")
	}

	c := ComputeExtensionHash([]string{"redis"})
	if a == c {
		t.Error("different extensions should produce different hash")
	}
}

func TestBuiltInProfiles(t *testing.T) {
	profiles := BuiltInProfiles()
	if len(profiles) < 4 {
		t.Errorf("expected at least 4 profiles, got %d", len(profiles))
	}

	names := make(map[string]bool)
	for _, p := range profiles {
		if names[p.Name] {
			t.Errorf("duplicate profile name: %s", p.Name)
		}
		names[p.Name] = true
	}
}

func TestVersionSelection(t *testing.T) {
	runtimes := []Runtime{
		{Version: "8.2.10", Platform: "linux", Arch: "amd64"},
		{Version: "8.3.6", Platform: "linux", Arch: "amd64"},
		{Version: "8.3.7", Platform: "linux", Arch: "amd64"},
		{Version: "8.4.0", Platform: "linux", Arch: "amd64"},
	}

	// Exact.
	rt, err := SelectVersion(runtimes, "8.3.6", PolicyExact)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Version != "8.3.6" {
		t.Errorf("exact: got %q, want %q", rt.Version, "8.3.6")
	}

	// Patch.
	rt, err = SelectVersion(runtimes, "8.3.6", PolicyPatch)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Version != "8.3.6" {
		t.Errorf("patch: got %q, want %q", rt.Version, "8.3.6")
	}

	// Minor (latest 8.3).
	rt, err = SelectVersion(runtimes, "8.3.0", PolicyMinor)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Version != "8.3.7" {
		t.Errorf("minor: got %q, want %q", rt.Version, "8.3.7")
	}

	// Locked.
	rt, err = SelectVersion(runtimes, "8.2.10", PolicyLocked)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Version != "8.2.10" {
		t.Errorf("locked: got %q, want %q", rt.Version, "8.2.10")
	}

	// No match.
	_, err = SelectVersion(runtimes, "9.0.0", PolicyExact)
	if err == nil {
		t.Error("expected error for no exact match")
	}
}
