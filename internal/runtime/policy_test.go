package runtime

import (
	"testing"
)

func TestSelectVersion_Policies(t *testing.T) {
	available := []Runtime{
		{Version: "8.1.10"},
		{Version: "8.2.5"},
		{Version: "8.3.1"},
		{Version: "8.3.4"},
	}

	// PolicyExact
	r, err := SelectVersion(available, "8.3.1", PolicyExact)
	if err != nil || r.Version != "8.3.1" {
		t.Errorf("PolicyExact failed: %v, %v", r, err)
	}

	// PolicyMinor
	r, err = SelectVersion(available, "8.3.0", PolicyMinor)
	if err != nil || r.Version != "8.3.4" {
		t.Errorf("PolicyMinor failed: got %v, want 8.3.4 (latest minor)", r)
	}

	// PolicyPatch
	r, err = SelectVersion(available, "8.3.1", PolicyPatch)
	if err != nil || r.Version != "8.3.1" {
		t.Errorf("PolicyPatch failed: got %v, want 8.3.1", r)
	}

	// PolicyLocked
	r, err = SelectVersion(available, "8.2.5", PolicyLocked)
	if err != nil || r.Version != "8.2.5" {
		t.Errorf("PolicyLocked failed: got %v, want 8.2.5", r)
	}
}

func TestVersionCompare(t *testing.T) {
	if compareVersions("8.3.4", "8.3.1") <= 0 {
		t.Error("expected 8.3.4 > 8.3.1")
	}
	if compareVersions("8.2.0", "8.3.0") >= 0 {
		t.Error("expected 8.2.0 < 8.3.0")
	}
	if compareVersions("8.3.0", "8.3.0") != 0 {
		t.Error("expected 8.3.0 == 8.3.0")
	}
}
