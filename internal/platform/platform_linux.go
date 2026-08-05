//go:build linux

package platform

import (
	"fmt"
	"net"
	"syscall"
	"time"
)

// SetProcessLimits configures OS file descriptor limits (rlimit).
func SetProcessLimits(maxFiles uint64) error {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return fmt.Errorf("getrlimit: %w", err)
	}

	if maxFiles > rLimit.Max {
		maxFiles = rLimit.Max
	}

	rLimit.Cur = maxFiles
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return fmt.Errorf("setrlimit: %w", err)
	}
	return nil
}

// IsSocketAvailable checks if a unix socket or TCP address is available for listening.
func IsSocketAvailable(network, addr string) bool {
	l, err := net.Listen(network, addr)
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// SendSignal sends an OS signal to a process.
func SendSignal(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

// DefaultFastCGISocket returns the default Linux PHP-FPM socket path.
func DefaultFastCGISocket() string {
	candidates := []string{
		"/run/php/php-fpm.sock",
		"/run/php/php8.3-fpm.sock",
		"/var/run/php5-fpm.sock",
	}
	for _, c := range candidates {
		if conn, err := net.DialTimeout("unix", c, 100*time.Millisecond); err == nil {
			conn.Close()
			return c
		}
	}
	return "/run/php/php-fpm.sock"
}
