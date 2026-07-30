// Package supervisor manages PHP-FPM process lifecycle.
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
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
	RuntimeDir     string       // Path to the runtime directory (for conf.d/)
	Extensions     []Extension  // Enabled extensions
	PhpIni         []IniSetting // Custom php.ini directives
}

// Extension represents a resolved PHP extension.
type Extension struct {
	Name string
	Type string // "extension" or "zend_extension"
}

// IniSetting represents a php.ini directive.
type IniSetting struct {
	Name  string
	Value string
}

// Supervisor manages a PHP-FPM process.
type Supervisor struct {
	cfg    Config
	cmd    *exec.Cmd
	mu     sync.Mutex
	state  State
	cancel context.CancelFunc

	// exited is closed by the reaper goroutine when the child process is
	// observed to exit, and exitErr holds why. Without this, a php-fpm that
	// dies leaves the state at StateReady forever and the gateway keeps
	// dialing a socket nobody is listening on.
	exited  chan struct{}
	exitErr error

	// isolator, when set, applies OS-level isolation to the child before it
	// starts.
	isolator *Isolator

	// lastFailure records why the last start or health check failed, for
	// operator-facing status.
	lastFailure error
}

// New creates a new Supervisor.
func New(cfg Config) *Supervisor {
	return &Supervisor{cfg: cfg, state: StateAbsent}
}

// SetIsolator installs OS-level isolation applied to php-fpm at start.
//
// §28.1 is explicit that isolation is tiered and that the project must not
// claim safe untrusted multi-tenancy at the lower tiers. A nil isolator means
// Tier 0: no isolation.
func (s *Supervisor) SetIsolator(iso *Isolator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isolator = iso
}

// LastFailure returns the most recent start or health-check failure, or nil.
func (s *Supervisor) LastFailure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastFailure
}

// setFailure records a failure reason alongside a state transition.
func (s *Supervisor) setFailure(state State, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.lastFailure = err
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

	// The caller's ctx bounds how long we wait for readiness. The child's
	// lifetime must NOT be tied to it: with exec.CommandContext(ctx, ...) and a
	// caller passing context.WithTimeout(..., 10*time.Second), php-fpm was
	// killed ten seconds after every successful start. Stop owns the kill
	// switch instead.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.cancel = cancel

	cmd := exec.CommandContext(runCtx, s.cfg.PHPBinary, "--fpm-config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Apply OS isolation before the process exists. This is the call that was
	// missing: internal/supervisor/isolation.go was fully implemented, tested,
	// and referenced by nothing, so the documented isolation tiers shipped as
	// unreachable code.
	s.mu.Lock()
	iso := s.isolator
	s.mu.Unlock()
	if iso != nil {
		if err := iso.ApplyIsolation(cmd); err != nil {
			cancel()
			s.setFailure(StateFailed, err)
			return fmt.Errorf("supervisor: apply isolation: %w", err)
		}
	}

	s.mu.Lock()
	s.cmd = cmd
	exited := make(chan struct{})
	s.exited = exited
	s.exitErr = nil
	s.mu.Unlock()

	if err := cmd.Start(); err != nil {
		cancel()
		s.setFailure(StateFailed, err)
		return fmt.Errorf("supervisor: start php-fpm: %w", err)
	}

	// One reaper per process. Wait must be called exactly once, so Stop and
	// HealthCheck both consult this instead of calling Wait themselves.
	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		s.exitErr = err
		s.mu.Unlock()
		close(exited)
	}()

	// Wait for socket to appear.
	if err := s.waitForSocket(ctx, 10*time.Second); err != nil {
		s.Stop(context.Background())
		s.setFailure(StateFailed, err)
		return fmt.Errorf("supervisor: wait for socket: %w", err)
	}

	s.mu.Lock()
	s.state = StateReady
	s.lastFailure = nil
	s.mu.Unlock()
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

	s.mu.Lock()
	cmd, exited := s.cmd, s.exited
	s.mu.Unlock()

	if cmd != nil && cmd.Process != nil && exited != nil {
		// Ask php-fpm to shut down gracefully first. SIGTERM lets the master
		// drain and reap its workers; going straight to SIGKILL (which is what
		// cancelling the exec context does) orphans them (§16.6).
		_ = cmd.Process.Signal(syscall.SIGTERM)

		// The reaper started in Start owns cmd.Wait; waiting on its channel
		// here avoids a second Wait, which would return an error and leave the
		// child unreaped.
		select {
		case <-exited:
		case <-ctx.Done():
			// Graceful shutdown ran out of time; force it.
			if s.cancel != nil {
				s.cancel()
			}
			_ = cmd.Process.Kill()
			<-exited
		}
	}

	// Release the exec context regardless of which path we took.
	if s.cancel != nil {
		s.cancel()
	}

	// Clean up socket and PID files.
	os.Remove(s.cfg.SocketPath)
	os.Remove(s.cfg.PIDFile)

	s.setState(StateStopped)
	return nil
}

// HealthCheck verifies the FPM process is alive and the socket is responsive.
//
// §16.4 lists seven readiness checks; this implements the first three (child
// alive, socket exists, socket accepts a connection). The remaining four —
// socket ownership, a minimal FastCGI health request, PHP version match, and
// extension verification — are tracked in ROADMAP.md.
func (s *Supervisor) HealthCheck(ctx context.Context) error {
	s.mu.Lock()
	state := s.state
	exited := s.exited
	s.mu.Unlock()

	if state != StateReady {
		return fmt.Errorf("supervisor: not ready (state %s)", state)
	}

	// Has the child died? "A process existing is not sufficient" (§16.4), and
	// neither is a process that has stopped existing going unnoticed.
	if exited != nil {
		select {
		case <-exited:
			// Read the reason only once exit is observed; reading it earlier
			// could catch a nil written before the reaper stored the error.
			s.mu.Lock()
			exitErr := s.exitErr
			s.mu.Unlock()
			if exitErr != nil {
				return fmt.Errorf("supervisor: php-fpm exited: %w", exitErr)
			}
			return errors.New("supervisor: php-fpm exited")
		default:
		}
	}

	// Check socket exists.
	info, err := os.Stat(s.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("supervisor: socket missing: %w", err)
	}
	if info == nil {
		return errors.New("supervisor: socket stat nil")
	}

	// A socket file that nobody is accepting on looks identical to a healthy
	// one from Stat alone, so actually connect.
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", s.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("supervisor: socket not accepting: %w", err)
	}
	conn.Close()

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

// knownBuiltinExtensions lists PHP extensions that are compiled into PHP
// and should not be loaded as shared objects (.so).
var knownBuiltinExtensions = map[string]bool{
	"core": true, "standard": true, "date": true, "json": true,
	"xml": true, "xmlwriter": true, "tokenizer": true, "dom": true,
	"libxml": true, "spl": true, "pcre": true, "filter": true,
	"hash": true, "random": true, "ctype": true,
}

// extensionLoadOrder defines dependencies that must load before their dependents.
// Entries with a lower priority value load first. Core/dependency extensions
// get priority 10, loadable extensions get priority 20.
var extensionLoadPriority = map[string]int{
	"mysqlnd":    1,
	"pdo":        2,
	"sqlite3":    3,
	"mysqli":     10,
	"pdo_mysql":  11,
	"pdo_sqlite": 12,
}

// sortExtensionsByDependency sorts extensions so that dependencies load first.
func sortExtensionsByDependency(extensions []Extension) []Extension {
	sorted := make([]Extension, len(extensions))
	copy(sorted, extensions)
	sort.SliceStable(sorted, func(i, j int) bool {
		pi := extensionLoadPriority[sorted[i].Name]
		pj := extensionLoadPriority[sorted[j].Name]
		if pi != 0 || pj != 0 {
			if pi == 0 {
				pi = 20
			}
			if pj == 0 {
				pj = 20
			}
			return pi < pj
		}
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
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

	var b []byte
	b = append(b, fmt.Sprintf(`[global]
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
	)...)

	// Add custom php.ini directives as php_admin_value.
	seen := make(map[string]bool)
	for _, ini := range s.cfg.PhpIni {
		if ini.Name == "" || seen[ini.Name] {
			continue
		}
		seen[ini.Name] = true
		b = append(b, fmt.Sprintf("php_admin_value[%s] = %s\n", ini.Name, ini.Value)...)
	}

	if s.cfg.Extensions != nil && len(s.cfg.Extensions) > 0 {
		b = append(b, "\n; Extensions loaded from resolved config\n"...)
		for _, ext := range sortExtensionsByDependency(s.cfg.Extensions) {
			if knownBuiltinExtensions[ext.Name] {
				continue
			}
			switch ext.Type {
			case "zend_extension":
				b = append(b, fmt.Sprintf("php_admin_value[zend_extension] = %s.so\n", ext.Name)...)
			default:
				b = append(b, fmt.Sprintf("php_admin_value[extension] = %s.so\n", ext.Name)...)
			}
		}
	}

	if err := os.WriteFile(cfgPath, b, 0o644); err != nil {
		return "", err
	}

	return cfgPath, nil
}
