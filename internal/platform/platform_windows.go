//go:build windows

package platform

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

// SetProcessLimits is a no-op on Windows.
func SetProcessLimits(maxFiles uint64) error {
	return nil
}

// IsSocketAvailable checks if a TCP address is available for listening on Windows.
func IsSocketAvailable(network, addr string) bool {
	l, err := net.Listen(network, addr)
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// SendSignal sends a process signal on Windows.
func SendSignal(pid int, sig syscall.Signal) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	return proc.Kill()
}

// DefaultFastCGISocket returns default Windows FastCGI address.
func DefaultFastCGISocket() string {
	return "127.0.0.1:9000"
}
