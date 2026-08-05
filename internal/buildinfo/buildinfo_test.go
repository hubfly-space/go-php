package buildinfo

import (
	"testing"
)

func TestBuildInfo_Get(t *testing.T) {
	info := Get()
	if info.Version == "" {
		t.Error("expected non-empty Version")
	}
	if info.GoVersion == "" {
		t.Error("expected non-empty GoVersion")
	}
}

func TestBuildInfo_BuildTime(t *testing.T) {
	// Default is "unknown" which returns zero Time
	bt := BuildTime()
	if !bt.IsZero() {
		t.Errorf("expected zero time for unknown BuildDate, got %v", bt)
	}
}
