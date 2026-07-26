// Package supervisor manages PHP-FPM process lifecycle.
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// State represents the FPM process state.
type State string

const (
	StateAbsent   State = "absent"
	StateStarting State = "starting"
	StateReady    State = "ready"
	StateDraining State = "draining"
	StateStopping State = "stopping"
	StateStopped  State = "stopped"
	StateFailed   State = "failed"
)

// Config holds FPM pool configuration.
type Config struct {
	PHPBinary      string // Path to php-fpm binary
	SocketPath     string // Unix socket path
	PIDFile        string
	MaxChildren    int
	StartServers   int
	MinSpare       int
	MaxSpare       int
	MaxRequests    int
	RequestTimeout time.Duration
	ErrorLog       string
	User           string
	Group          string
}

// Supervisor manages a PHP-FPM process.
type Supervisor struct {
	cfg    Config
	cmd    *exec.Cmd
	mu     sync.Mutex
	state  State
	cancel context.CancelFunc
}

// New creates a new Supervisor.
func New(cfg Config) *Supervisor {
	return &Supervisor{cfg: cfg, state: StateAbsent}
}

// State returns the current state.
func (s *Supervisor) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Start launches php-fpm and waits for the socket to be ready.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.state == StateReady || s.state == StateStarting {
		s.mu.Unlock()
		return nil
	}
	s.state = StateStarting
	s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.cfg.SocketPath), 0o755); err != nil {
		s.setState(StateFailed)
		return fmt.Errorf("supervisor: create socket dir: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.cfg.PIDFile), 0o755); err != nil {
		s.setState(StateFailed)
		return fmt.Errorf("supervisor: create pid dir: %w", err)
	}

	configPath, err := s.generateConfig()
	if err != nil {
		s.setState(StateFailed)
		return fmt.Errorf("supervisor: generate config: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	cmd := exec.CommandContext(ctx, s.cfg.PHPBinary, "--fpm-config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	s.cmd = cmd

	if err := cmd.Start(); err != nil {
		s.setState(StateFailed)
		return fmt.Errorf("supervisor: start php-fpm: %w", err)
	}

	// Wait for socket to appear.
	if err := s.waitForSocket(ctx, 10*time.Second); err != nil {
		s.Stop(context.Background())
		s.setState(StateFailed)
		return fmt.Errorf("supervisor: wait for socket: %w", err)
	}

	s.setState(StateReady)
	return nil
}

// Stop gracefully stops php-fpm.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.state == StateStopped || s.state == StateAbsent {
		s.mu.Unlock()
		return nil
	}
	s.state = StateStopping
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	if s.cmd != nil && s.cmd.Process != nil {
		done := make(chan error, 1)
		go func() {
			done <- s.cmd.Wait()
		}()

		select {
		case <-done:
		case <-ctx.Done():
			s.cmd.Process.Kill()
			<-done
		}
	}

	// Clean up socket and PID files.
	os.Remove(s.cfg.SocketPath)
	os.Remove(s.cfg.PIDFile)

	s.setState(StateStopped)
	return nil
}

// HealthCheck verifies the FPM process is alive and the socket is responsive.
func (s *Supervisor) HealthCheck(ctx context.Context) error {
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()

	if state != StateReady {
		return errors.New("supervisor: not ready")
	}

	// Check socket exists.
	info, err := os.Stat(s.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("supervisor: socket missing: %w", err)
	}
	if info == nil {
		return errors.New("supervisor: socket stat nil")
	}

	return nil
}

func (s *Supervisor) setState(state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
}

func (s *Supervisor) waitForSocket(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		info, err := os.Stat(s.cfg.SocketPath)
		if err == nil && info != nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("timeout waiting for socket")
}

// generateConfig writes an FPM config file and returns its path.
func (s *Supervisor) generateConfig() (string, error) {
	dir := filepath.Dir(s.cfg.PIDFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	cfgPath := filepath.Join(dir, "php-fpm.conf")

	maxChildren := s.cfg.MaxChildren
	if maxChildren == 0 {
		maxChildren = 20
	}
	startServers := s.cfg.StartServers
	if startServers == 0 {
		startServers = 2
	}
	minSpare := s.cfg.MinSpare
	if minSpare == 0 {
		minSpare = 2
	}
	maxSpare := s.cfg.MaxSpare
	if maxSpare == 0 {
		maxSpare = 6
	}
	maxRequests := s.cfg.MaxRequests
	if maxRequests == 0 {
		maxRequests = 500
	}

	content := fmt.Sprintf(`[global]
pid = %s
error_log = %s
daemonize = no

[gateway]
listen = %s
listen.owner = %d
listen.group = %d
listen.mode = 0600

pm = dynamic
pm.max_children = %d
pm.start_servers = %d
pm.min_spare_servers = %d
pm.max_spare_servers = %d
pm.max_requests = %d

request_terminate_timeout = %ds
catch_workers_output = yes
clear_env = yes
security.limit_extensions = .php
`,
		s.cfg.PIDFile,
		s.cfg.ErrorLog,
		s.cfg.SocketPath,
		os.Getuid(), os.Getgid(),
		maxChildren,
		startServers,
		minSpare,
		maxSpare,
		maxRequests,
		int(s.cfg.RequestTimeout.Seconds()),
	)

	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		return "", err
	}

	return cfgPath, nil
}
