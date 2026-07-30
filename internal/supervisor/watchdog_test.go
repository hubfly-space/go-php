package supervisor

import (
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{
		Interval:        5 * time.Millisecond,
		MinBackoff:      time.Millisecond,
		MaxBackoff:      2 * time.Millisecond,
		MaxRestarts:     3,
		RestartWindow:   time.Minute,
		CircuitCooldown: time.Minute,
	}
}

func TestWatchdogRunExitsOnCancel(t *testing.T) {
	sup := New(Config{PHPBinary: "/nonexistent", SocketPath: t.TempDir() + "/s.sock"})
	w := NewWatchdog(sup, testWatchdogConfig(), slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	done := w.Run(ctx)
	cancel()

	select {
	case <-done:
		// §62 requires every goroutine to have a test proving it exits.
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog goroutine did not exit after context cancellation")
	}
}

func TestWatchdogIgnoresStoppedSupervisor(t *testing.T) {
	// A supervisor that was never started must not be "restarted" into
	// existence by the watchdog.
	sup := New(Config{PHPBinary: "/nonexistent", SocketPath: t.TempDir() + "/s.sock"})
	if sup.State() != StateAbsent {
		t.Fatalf("state = %s, want absent", sup.State())
	}

	w := NewWatchdog(sup, testWatchdogConfig(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := w.Run(ctx)

	time.Sleep(50 * time.Millisecond)

	if got := w.Restarts(); got != 0 {
		t.Errorf("restarts = %d, want 0 for an unstarted supervisor", got)
	}
	if sup.State() != StateAbsent {
		t.Errorf("state = %s, want absent", sup.State())
	}

	cancel()
	<-done
}

func TestWatchdogOpensCircuitOnRepeatedFailure(t *testing.T) {
	// Force the supervisor into StateReady with a socket that does not exist,
	// so every health check fails and every restart attempt fails too.
	sup := New(Config{
		PHPBinary:  "/nonexistent/php-fpm",
		SocketPath: t.TempDir() + "/missing.sock",
		PIDFile:    t.TempDir() + "/fpm.pid",
	})
	sup.setState(StateReady)

	cfg := testWatchdogConfig()
	w := NewWatchdog(sup, cfg, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := w.Run(ctx)

	// §16.5: "Never loop rapidly and consume the host." The circuit must open
	// rather than the loop retrying forever.
	deadline := time.After(5 * time.Second)
	for !w.CircuitOpen() {
		select {
		case <-deadline:
			t.Fatalf("circuit never opened after %d restarts", w.Restarts())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	if got := w.Restarts(); got > cfg.MaxRestarts {
		t.Errorf("restarts = %d, want <= MaxRestarts %d", got, cfg.MaxRestarts)
	}

	// Once open, the watchdog must stop attempting restarts.
	before := w.Restarts()
	time.Sleep(100 * time.Millisecond)
	if after := w.Restarts(); after != before {
		t.Errorf("restarts grew from %d to %d while the circuit was open", before, after)
	}

	cancel()
	<-done
}

func TestWatchdogBackoffGrowsAndIsBounded(t *testing.T) {
	cfg := WatchdogConfig{
		Interval:      time.Second,
		MinBackoff:    100 * time.Millisecond,
		MaxBackoff:    time.Second,
		MaxRestarts:   10,
		RestartWindow: time.Minute,
	}
	w := NewWatchdog(New(Config{}), cfg, slog.Default())

	// Jitter is over [d/2, d], so compare against the floor of each step.
	for attempt := 0; attempt < 8; attempt++ {
		got := w.backoff(attempt)
		if got < cfg.MinBackoff/2 {
			t.Errorf("backoff(%d) = %v, below the minimum floor", attempt, got)
		}
		if got > cfg.MaxBackoff {
			t.Errorf("backoff(%d) = %v, exceeds MaxBackoff %v", attempt, got, cfg.MaxBackoff)
		}
	}

	// Jitter must actually vary, or several supervisors would retry in lockstep.
	seen := make(map[time.Duration]bool)
	for i := 0; i < 20; i++ {
		seen[w.backoff(3)] = true
	}
	if len(seen) < 2 {
		t.Error("backoff produced no jitter across 20 samples")
	}
}

func TestHealthCheckDetectsExitedProcess(t *testing.T) {
	dir := t.TempDir()
	sup := New(Config{
		PHPBinary:  "/bin/true", // exits immediately
		SocketPath: dir + "/s.sock",
		PIDFile:    dir + "/fpm.pid",
	})

	// Start fails (no socket ever appears), which is itself correct.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := sup.Start(ctx); err == nil {
		t.Fatal("expected Start to fail when the process exits without creating a socket")
	}

	if sup.State() != StateFailed && sup.State() != StateStopped {
		t.Errorf("state = %s, want failed or stopped", sup.State())
	}
	if sup.LastFailure() == nil {
		t.Error("LastFailure should record why the start failed")
	}
}

func TestStartDoesNotTieProcessLifetimeToStartupDeadline(t *testing.T) {
	// Regression: Start used exec.CommandContext with the caller's context, so
	// a caller passing context.WithTimeout(..., 10s) — as cmd/gateway does —
	// had php-fpm killed ten seconds after a successful start. Nothing noticed,
	// because the state stayed StateReady.
	dir := t.TempDir()
	sock := filepath.Join(dir, "s.sock")

	// Stand in for php-fpm: a long-lived process, plus a socket so readiness
	// succeeds.
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	// Start always passes "--fpm-config <path>", which /bin/sleep rejects, so
	// use a script that ignores its arguments and stays alive.
	fakeFPM := filepath.Join(dir, "fake-fpm")
	if err := os.WriteFile(fakeFPM, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatalf("write fake fpm: %v", err)
	}

	sup := New(Config{
		PHPBinary:  fakeFPM,
		SocketPath: sock,
		PIDFile:    filepath.Join(dir, "fpm.pid"),
	})

	// A short startup deadline, of the kind the caller passes.
	startCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := sup.Start(startCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let the startup deadline lapse.
	<-startCtx.Done()
	time.Sleep(200 * time.Millisecond)

	sup.mu.Lock()
	exited := sup.exited
	sup.mu.Unlock()

	select {
	case <-exited:
		t.Fatal("child was killed when the startup deadline expired; its lifetime must be independent")
	default:
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sup.Stop(stopCtx)
}
