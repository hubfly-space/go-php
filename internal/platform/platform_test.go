package platform

import (
	"net"
	"testing"
)

func TestCurrentInfo(t *testing.T) {
	info := CurrentInfo()
	if info.OS == "" {
		t.Error("expected non-empty OS")
	}
	if info.Arch == "" {
		t.Error("expected non-empty Arch")
	}
	if info.NumCPU <= 0 {
		t.Errorf("expected positive NumCPU, got %d", info.NumCPU)
	}
}

func TestSocketAvailability(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := l.Addr().String()

	// Port is currently in use by l
	if IsSocketAvailable("tcp", addr) {
		t.Errorf("expected port %s to be unavailable while listener is active", addr)
	}

	l.Close()

	// Port should now be available
	if !IsSocketAvailable("tcp", addr) {
		t.Errorf("expected port %s to be available after listener closed", addr)
	}
}
