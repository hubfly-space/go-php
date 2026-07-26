package diagnostics

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Doctor performs system health checks for the gateway.
type Doctor struct {
	binaries []string
	ports    []int
}

// NewDoctor creates a new system doctor.
func NewDoctor() *Doctor {
	return &Doctor{
		binaries: []string{"php-fpm", "php", "composer"},
		ports:    []int{80, 443, 9090},
	}
}

// CheckResult is a single health check result.
type CheckResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // ok, warn, fail
	Message string `json:"message"`
}

// DoctorReport holds the full system health check report.
type DoctorReport struct {
	Checks   []CheckResult `json:"checks"`
	OS       string        `json:"os"`
	Arch     string        `json:"arch"`
	GoVer    string        `json:"go_version"`
	Hostname string        `json:"hostname"`
}

// Run performs all health checks.
func (d *Doctor) Run() *DoctorReport {
	report := &DoctorReport{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		GoVer:    runtime.Version(),
		Hostname: hostname(),
	}

	d.checkBinaries(report)
	d.checkPorts(report)
	d.checkPIDLimit(report)
	d.checkOpenFiles(report)
	d.checkDiskSpace(report)

	return report
}

func (d *Doctor) checkBinaries(r *DoctorReport) {
	for _, bin := range d.binaries {
		path, err := exec.LookPath(bin)
		if err != nil {
			r.Checks = append(r.Checks, CheckResult{
				Name:    "binary:" + bin,
				Status:  "warn",
				Message: fmt.Sprintf("%s not found in PATH", bin),
			})
			continue
		}

		// Get version if possible.
		r.Checks = append(r.Checks, CheckResult{
			Name:    "binary:" + bin,
			Status:  "ok",
			Message: fmt.Sprintf("found at %s", path),
		})
	}
}

func (d *Doctor) checkPorts(r *DoctorReport) {
	for _, port := range d.ports {
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			r.Checks = append(r.Checks, CheckResult{
				Name:    fmt.Sprintf("port:%d", port),
				Status:  "warn",
				Message: fmt.Sprintf("port %d in use", port),
			})
			continue
		}
		ln.Close()
		r.Checks = append(r.Checks, CheckResult{
			Name:    fmt.Sprintf("port:%d", port),
			Status:  "ok",
			Message: fmt.Sprintf("port %d available", port),
		})
	}
}

func (d *Doctor) checkPIDLimit(r *DoctorReport) {
	// Check /proc/sys/kernel/pid_max on Linux.
	if runtime.GOOS != "linux" {
		return
	}

	data, err := os.ReadFile("/proc/sys/kernel/pid_max")
	if err != nil {
		return
	}

	pidMax := strings.TrimSpace(string(data))
	r.Checks = append(r.Checks, CheckResult{
		Name:    "system:pid_max",
		Status:  "ok",
		Message: fmt.Sprintf("pid_max = %s", pidMax),
	})
}

func (d *Doctor) checkOpenFiles(r *DoctorReport) {
	if runtime.GOOS != "linux" {
		return
	}

	// Check /proc/self/limits for open files.
	data, err := os.ReadFile("/proc/self/limits")
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "Max open files") {
			r.Checks = append(r.Checks, CheckResult{
				Name:    "system:open_files",
				Status:  "ok",
				Message: strings.TrimSpace(line),
			})
			return
		}
	}
}

func (d *Doctor) checkDiskSpace(r *DoctorReport) {
	// Check if we can write to the temp directory.
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, ".gateway-doctor-test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		r.Checks = append(r.Checks, CheckResult{
			Name:    "system:disk",
			Status:  "warn",
			Message: fmt.Sprintf("cannot write to temp dir: %v", err),
		})
		return
	}
	os.Remove(testFile)

	r.Checks = append(r.Checks, CheckResult{
		Name:    "system:disk",
		Status:  "ok",
		Message: "temp directory writable",
	})
}

// Summary returns a human-readable summary of the report.
func (r *DoctorReport) Summary() string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Gateway Doctor Report (%s)", time.Now().Format(time.RFC3339)))
	lines = append(lines, fmt.Sprintf("OS: %s/%s  Go: %s  Host: %s", r.OS, r.Arch, r.GoVer, r.Hostname))
	lines = append(lines, "")

	for _, c := range r.Checks {
		status := "[OK]"
		switch c.Status {
		case "warn":
			status = "[WARN]"
		case "fail":
			status = "[FAIL]"
		}
		lines = append(lines, fmt.Sprintf("  %s %-20s %s", status, c.Name, c.Message))
	}

	return strings.Join(lines, "\n")
}

// HasFailures returns true if any check failed.
func (r *DoctorReport) HasFailures() bool {
	for _, c := range r.Checks {
		if c.Status == "fail" {
			return true
		}
	}
	return false
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
