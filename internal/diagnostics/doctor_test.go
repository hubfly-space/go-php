package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDoctor_Run(t *testing.T) {
	doctor := NewDoctor()
	report := doctor.Run()

	if report.OS == "" {
		t.Error("expected OS to be set")
	}
	if report.Arch == "" {
		t.Error("expected Arch to be set")
	}
	if report.Hostname == "" {
		t.Error("expected Hostname to be set")
	}
	if len(report.Checks) == 0 {
		t.Error("expected at least one check")
	}

	t.Logf("report:\n%s", report.Summary())
}

func TestDoctor_ReportSummary(t *testing.T) {
	report := &DoctorReport{
		OS:       "linux",
		Arch:     "amd64",
		GoVer:    "go1.25.3",
		Hostname: "test-host",
		Checks: []CheckResult{
			{Name: "binary:php", Status: "ok", Message: "found at /usr/bin/php"},
			{Name: "port:80", Status: "warn", Message: "port 80 in use"},
		},
	}

	summary := report.Summary()
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("summary:\n%s", summary)
}

func TestDoctorReport_HasFailures(t *testing.T) {
	report := &DoctorReport{
		Checks: []CheckResult{
			{Status: "ok"},
			{Status: "warn"},
		},
	}

	if report.HasFailures() {
		t.Error("expected no failures")
	}

	report.Checks = append(report.Checks, CheckResult{Status: "fail"})
	if !report.HasFailures() {
		t.Error("expected failures")
	}
}

func TestDoctor_Binaries(t *testing.T) {
	doctor := NewDoctor()
	report := doctor.Run()

	// At least check that the check results are well-formed.
	for _, c := range report.Checks {
		if c.Name == "" {
			t.Error("expected non-empty check name")
		}
		if c.Status != "ok" && c.Status != "warn" && c.Status != "fail" {
			t.Errorf("invalid status %q for check %s", c.Status, c.Name)
		}
	}
}

func TestDoctor_Ports(t *testing.T) {
	doctor := NewDoctor()
	report := doctor.Run()

	// Check that port checks are present.
	foundPortCheck := false
	for _, c := range report.Checks {
		if len(c.Name) > 5 && c.Name[:5] == "port:" {
			foundPortCheck = true
			break
		}
	}
	if !foundPortCheck {
		t.Error("expected at least one port check")
	}
}

func TestDoctor_SummaryContainsHostname(t *testing.T) {
	report := &DoctorReport{
		OS:       "linux",
		Arch:     "amd64",
		GoVer:    "go1.25.3",
		Hostname: "my-host",
	}

	summary := report.Summary()
	if !containsStr(summary, "my-host") {
		t.Error("expected summary to contain hostname")
	}
}

func TestDoctor_EmptyReport(t *testing.T) {
	report := &DoctorReport{
		Checks: []CheckResult{},
	}

	if report.HasFailures() {
		t.Error("expected no failures for empty report")
	}
}

func TestWritablePathsInTemp(t *testing.T) {
	dir := t.TempDir()

	// Create some directories manually to simulate a project.
	os.MkdirAll(filepath.Join(dir, "cache"), 0755)
	os.MkdirAll(filepath.Join(dir, "logs"), 0755)
	os.MkdirAll(filepath.Join(dir, "storage"), 0755)

	// Verify they exist.
	for _, d := range []string{"cache", "logs", "storage"} {
		path := filepath.Join(dir, d)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", d)
		}
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
