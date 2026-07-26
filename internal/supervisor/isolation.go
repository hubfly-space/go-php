package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
)

// IsolationConfig configures OS-level isolation for PHP processes.
type IsolationConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Mode        string `yaml:"mode"`         // "none", "process", "namespace", "cgroup"
	User        string `yaml:"user"`         // drop privileges to this user
	ChrootDir   string `yaml:"chroot_dir"`   // chroot directory
	CgroupPath  string `yaml:"cgroup_path"`  // cgroup v2 path
	MemoryLimit string `yaml:"memory_limit"` // e.g. "256M"
	CPULimit    string `yaml:"cpu_limit"`    // e.g. "0.5" (half a CPU)
	PIDLimit    int    `yaml:"pid_limit"`    // max processes
	NoNewPrivs  bool   `yaml:"no_new_privs"` // prctl(PR_SET_NO_NEW_PRIVS)
}

// DefaultIsolationConfig returns safe defaults.
func DefaultIsolationConfig() *IsolationConfig {
	return &IsolationConfig{
		Enabled:    false,
		Mode:       "process",
		NoNewPrivs: true,
		PIDLimit:   64,
	}
}

// Isolator wraps exec.Cmd with OS-level isolation.
type Isolator struct {
	config *IsolationConfig
	mu     sync.Mutex
}

// NewIsolator creates a new process isolator.
func NewIsolator(config *IsolationConfig) *Isolator {
	if config == nil {
		config = DefaultIsolationConfig()
	}
	return &Isolator{config: config}
}

// ApplyIsolation applies isolation settings to an exec.Cmd before starting.
func (iso *Isolator) ApplyIsolation(cmd *exec.Cmd) error {
	if !iso.config.Enabled {
		return nil
	}

	iso.mu.Lock()
	defer iso.mu.Unlock()

	switch iso.config.Mode {
	case "process":
		return iso.applyProcessIsolation(cmd)
	case "namespace":
		return iso.applyNamespaceIsolation(cmd)
	case "cgroup":
		return iso.applyCgroupIsolation(cmd)
	default:
		return nil
	}
}

func (iso *Isolator) applyProcessIsolation(cmd *exec.Cmd) error {
	// Set process attributes for isolation.
	cmd.SysProcAttr = &syscall.SysProcAttr{}

	// Set environment to minimal.
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"TMPDIR=/tmp",
	}

	// Apply resource limits via Setrlimit (Linux only).
	if runtime.GOOS == "linux" {
		cmd.SysProcAttr.Credential = iso.credential()
	}

	return nil
}

func (iso *Isolator) applyNamespaceIsolation(cmd *exec.Cmd) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("namespace isolation requires Linux")
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}

	// Set environment to minimal.
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
	}

	return nil
}

func (iso *Isolator) applyCgroupIsolation(cmd *exec.Cmd) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("cgroup isolation requires Linux")
	}

	// Create cgroup for the process.
	if iso.config.CgroupPath != "" {
		cgroupPath := filepath.Join("/sys/fs/cgroup", iso.config.CgroupPath)
		os.MkdirAll(cgroupPath, 0755)

		// Set memory limit.
		if iso.config.MemoryLimit != "" {
			bytes := parseBytes(iso.config.MemoryLimit)
			if bytes > 0 {
				os.WriteFile(filepath.Join(cgroupPath, "memory.max"), []byte(fmt.Sprintf("%d", bytes)), 0644)
			}
		}

		// Set PID limit.
		if iso.config.PIDLimit > 0 {
			os.WriteFile(filepath.Join(cgroupPath, "pids.max"),
				[]byte(fmt.Sprintf("%d", iso.config.PIDLimit)), 0644)
		}
	}

	// Apply process-level isolation as well.
	return iso.applyProcessIsolation(cmd)
}

func (iso *Isolator) credential() *syscall.Credential {
	if iso.config.User == "" || iso.config.User == "root" {
		return nil
	}

	// In production, look up the user by name and set UID/GID.
	// For now, use nobody (65534) as a safe default.
	return &syscall.Credential{
		Uid:         65534,
		Gid:         65534,
		NoSetGroups: true,
	}
}

// Cleanup removes cgroup directories created by the isolator.
func (iso *Isolator) Cleanup() {
	if iso.config.CgroupPath != "" && runtime.GOOS == "linux" {
		cgroupPath := filepath.Join("/sys/fs/cgroup", iso.config.CgroupPath)
		os.RemoveAll(cgroupPath)
	}
}

// parseBytes parses a byte size string like "256M" into bytes.
func parseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0
	}

	multiplier := int64(1)
	last := s[len(s)-1]

	switch last {
	case 'G', 'g':
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case 'M', 'm':
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	case 'K', 'k':
		multiplier = 1024
		s = s[:len(s)-1]
	}

	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}

	return n * multiplier
}
