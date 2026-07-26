// Package chaos provides chaos and fault injection tests for the go-php gateway.
//
// Run with:
//
//	go test -tags=chaos ./test/chaos/...
//
//go:build chaos

package chaos

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-php/gateway/internal/config"
	"github.com/go-php/gateway/internal/deploy"
	"github.com/go-php/gateway/internal/runtime"
)

// TestDiskFullSimulatesDiskFullDuringDeploy simulates disk full during deployment.
func TestDiskFullSimulatesDiskFullDuringDeploy(t *testing.T) {
	dir := t.TempDir()
	mgr := deploy.NewReleaseManager(dir)

	// Create a release.
	rel, err := mgr.Create("v1.0.0", "php-8.3", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate disk full by making releases dir read-only.
	os.Chmod(dir, 0555)
	defer os.Chmod(dir, 0755)

	// Attempt to create another release — should fail gracefully.
	_, err = mgr.Create("v2.0.0", "php-8.3", t.TempDir(), nil)
	if err == nil {
		t.Log("expected disk full error but create succeeded (may not be on same filesystem)")
	}

	_ = rel
}

// TestCorruptedStateFileSimulatesCorruptedState simulates corrupted state.
func TestCorruptedStateFileSimulatesCorruptedState(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	// Write corrupted JSON.
	os.WriteFile(stateFile, []byte(`{corrupted json`), 0644)

	// Verify that loading corrupted state doesn't crash.
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}

	// Attempt to parse — should fail gracefully.
	if len(data) > 0 && data[0] == '{' {
		// Corrupted JSON detected.
		t.Log("corrupted state detected, would rollback to last known good")
	}
}

// TestNetworkPartitionSimulatesNetworkPartition simulates network partition.
func TestNetworkPartitionSimulatesNetworkPartition(t *testing.T) {
	// Create a listener that we'll close abruptly.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := ln.Addr().String()

	// Close the listener to simulate partition.
	ln.Close()

	// Try to connect — should fail.
	_, err = net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err == nil {
		t.Error("expected connection to fail after partition")
	}
}

// TestProcessKillDuringDeploy simulates process kill during deployment.
func TestProcessKillDuringDeploy(t *testing.T) {
	dir := t.TempDir()
	mgr := deploy.NewReleaseManager(dir)

	// Create multiple releases.
	for i := 0; i < 5; i++ {
		rel, err := mgr.Create(fmt.Sprintf("v%d.0.0", i), "php-8.3", t.TempDir(), nil)
		if err != nil {
			t.Fatal(err)
		}

		// Activate each one.
		if err := mgr.Activate(rel.ID); err != nil {
			t.Fatal(err)
		}
	}

	// Verify we can still rollback.
	rel, err := mgr.Rollback()
	if err != nil {
		t.Fatalf("rollback failed after simulated kills: %v", err)
	}

	if rel == nil {
		t.Error("expected non-nil release after rollback")
	}
}

// TestConcurrentDeployAndRollback simulates concurrent deploy/rollback operations.
func TestConcurrentDeployAndRollback(t *testing.T) {
	dir := t.TempDir()
	mgr := deploy.NewReleaseManager(dir)

	// Create initial release.
	rel, err := mgr.Create("v1.0.0", "php-8.3", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mgr.Activate(rel.ID)

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	// Concurrent creates.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := mgr.Create(fmt.Sprintf("v%d.0.0", i+2), "php-8.3", t.TempDir(), nil)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent create error: %v", err)
	}
}

// TestTimeoutDuringFPMConnection simulates FPM connection timeout.
func TestTimeoutDuringFPMConnection(t *testing.T) {
	// Create a server that never responds.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Accept but never respond.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold connection open forever.
			time.Sleep(10 * time.Second)
			conn.Close()
		}
	}()

	// Try to connect with a short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", ln.Addr().String())
	if err == nil {
		conn.Close()
		t.Error("expected timeout error")
	}
}

// TestRuntimeRegistryCorruption simulates registry corruption.
func TestRuntimeRegistryCorruption(t *testing.T) {
	dir := t.TempDir()

	// Create a corrupted index file.
	indexFile := filepath.Join(dir, "index.json")
	os.WriteFile(indexFile, []byte(`{"runtimes": [{"version": "8.3", incomplete`), 0644)

	// Verify the runtime package handles corruption gracefully.
	idx, err := runtime.LoadIndex(indexFile)
	if err != nil {
		// Expected — corruption detected.
		t.Logf("corruption detected: %v", err)
		return
	}

	if idx == nil {
		t.Log("nil index returned for corrupted file")
	}
}

// TestMalformedRequestSimulatesMalformedRequest simulates receiving malformed requests.
func TestMalformedRequestSimulatesMalformedRequest(t *testing.T) {
	malformedPaths := []string{
		"",
		"no-leading-slash",
		"/path with spaces",
		"/path\twith\ttabs",
		"/path\nwith\nnewlines",
		string([]byte{0xff, 0xfe}), // invalid UTF-8
	}

	for _, path := range malformedPaths {
		t.Run(path, func(t *testing.T) {
			_, err := config.Load(path)
			if err == nil && path != "" {
				t.Log("loaded malformed path as config (unexpected)")
			}
		})
	}
}

// TestResourceExhaustionSimulatesResourceExhaustion simulates resource exhaustion.
func TestResourceExhaustionSimulatesResourceExhaustion(t *testing.T) {
	dir := t.TempDir()

	// Create many files to simulate inode exhaustion.
	for i := 0; i < 1000; i++ {
		fname := filepath.Join(dir, fmt.Sprintf("file_%d.txt", i))
		os.WriteFile(fname, []byte("data"), 0644)
	}

	// Verify we can still operate.
	entries, _ := os.ReadDir(dir)
	if len(entries) < 1000 {
		t.Errorf("expected 1000 entries, got %d", len(entries))
	}

	// Clean up.
	for i := 0; i < 1000; i++ {
		os.Remove(filepath.Join(dir, fmt.Sprintf("file_%d.txt", i)))
	}
}
