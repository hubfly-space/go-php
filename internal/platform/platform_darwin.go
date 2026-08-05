//go:build darwin

package platform

import (
	"fmt"
	"net"
	"syscall"
	"time"
)

// SetProcessLimits configures OS file descriptor limits (rlimit) on macOS.
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

// IsSocketAvailable checks if a unix socket or TCP address is available for listening on macOS.
func IsSocketAvailable(network, addr string) bool {
	l, err := net.Listen(network, addr)
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// SendSignal sends an OS signal to a process on macOS.
func SendSignal(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

// DefaultFastCGISocket returns default macOS PHP-FPM socket path.
func DefaultFastCGISocket() string {
	candidates := []string{
		"/tmp/php-fpm.sock",
		"/usr/local/var/run/php-fpm.sock",
		"/opt/homebrew/var/run/php-fpm.sock",
	}
	for _, c := range candidates {
		if conn, err := net.DialTimeout("unix", c, 100*time.Millisecond); err == nil {
			conn.Close()
			return c
		}
	}
	return "127.0.0.1:9000"
}
