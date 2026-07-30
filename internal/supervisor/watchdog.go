package supervisor

import (
	"context"
	"crypto/rand"
	"log/slog"
	"math/big"
	"sync"
	"time"
)

// WatchdogConfig tunes the health-check and restart loop (§16.5).
type WatchdogConfig struct {
	// Interval between health checks.
	Interval time.Duration

	// MinBackoff and MaxBackoff bound the exponential restart delay.
	MinBackoff time.Duration
	MaxBackoff time.Duration

	// MaxRestarts is how many restarts may occur within RestartWindow before
	// the circuit opens. §16.5: "Never loop rapidly and consume the host."
	MaxRestarts   int
	RestartWindow time.Duration

	// CircuitCooldown is how long the circuit stays open before the watchdog
	// tries again.
	CircuitCooldown time.Duration
}

// DefaultWatchdogConfig returns conservative defaults.
func DefaultWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{
		Interval:        5 * time.Second,
		MinBackoff:      500 * time.Millisecond,
		MaxBackoff:      30 * time.Second,
		MaxRestarts:     5,
		RestartWindow:   time.Minute,
		CircuitCooldown: 2 * time.Minute,
	}
}

// Watchdog health-checks a Supervisor and restarts it with backoff.
type Watchdog struct {
	sup    *Supervisor
	cfg    WatchdogConfig
	logger *slog.Logger

	mu           sync.Mutex
	circuitOpen  bool
	circuitUntil time.Time
	restarts     []time.Time
	totalRestart int
}

// NewWatchdog creates a watchdog for a supervisor.
func NewWatchdog(sup *Supervisor, cfg WatchdogConfig, logger *slog.Logger) *Watchdog {
	if cfg.Interval <= 0 {
		cfg = DefaultWatchdogConfig()
	}
	return &Watchdog{sup: sup, cfg: cfg, logger: logger}
}

// CircuitOpen reports whether restarts are currently suspended.
//
// While the circuit is open the gateway should keep serving static files and
// return a stable 503 for PHP routes (§16.5) rather than failing in a new way
// on every request.
func (w *Watchdog) CircuitOpen() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.circuitOpen && time.Now().Before(w.circuitUntil)
}

// Restarts returns how many times the watchdog has restarted php-fpm.
func (w *Watchdog) Restarts() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.totalRestart
}

// Run health-checks on an interval until ctx is canceled, restarting php-fpm
// when a check fails. The returned channel closes when the goroutine exits.
func (w *Watchdog) Run(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(w.cfg.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.check(ctx)
			}
		}
	}()

	return done
}

func (w *Watchdog) check(ctx context.Context) {
	// A supervisor that was never started, or was deliberately stopped, is not
	// the watchdog's business.
	switch w.sup.State() {
	case StateStopped, StateStopping, StateAbsent, StateStarting, StateDraining:
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := w.sup.HealthCheck(checkCtx)
	cancel()
	if err == nil {
		w.recordHealthy()
		return
	}

	if w.CircuitOpen() {
		return
	}

	w.logger.Error("php-fpm health check failed", "error", err)
	w.restart(ctx)
}

// recordHealthy closes the circuit once the process is stable again.
func (w *Watchdog) recordHealthy() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.circuitOpen && time.Now().After(w.circuitUntil) {
		w.circuitOpen = false
		w.restarts = nil
		w.logger.Info("php-fpm recovered, restart circuit closed")
	}
}

// restart stops and restarts php-fpm with exponential backoff and jitter,
// opening the circuit if the restart rate is exceeded.
func (w *Watchdog) restart(ctx context.Context) {
	w.mu.Lock()

	// Drop restarts that have aged out of the window.
	cutoff := time.Now().Add(-w.cfg.RestartWindow)
	kept := w.restarts[:0]
	for _, t := range w.restarts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	w.restarts = kept

	if len(w.restarts) >= w.cfg.MaxRestarts {
		w.circuitOpen = true
		w.circuitUntil = time.Now().Add(w.cfg.CircuitCooldown)
		attempts := len(w.restarts)
		w.mu.Unlock()

		w.logger.Error("php-fpm restart circuit opened; PHP requests will fail until it closes",
			"restarts", attempts, "window", w.cfg.RestartWindow, "cooldown", w.cfg.CircuitCooldown)
		return
	}

	attempt := len(w.restarts)
	w.restarts = append(w.restarts, time.Now())
	w.totalRestart++
	w.mu.Unlock()

	delay := w.backoff(attempt)
	w.logger.Warn("restarting php-fpm", "attempt", attempt+1, "delay", delay)

	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_ = w.sup.Stop(stopCtx)
	cancel()

	startCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := w.sup.Start(startCtx); err != nil {
		w.logger.Error("php-fpm restart failed", "error", err)
		return
	}
	w.logger.Info("php-fpm restarted")
}

// backoff returns an exponentially increasing delay with jitter.
//
// Jitter matters even for a single process: without it, a supervisor and
// whatever it depends on retry in lockstep and keep colliding.
func (w *Watchdog) backoff(attempt int) time.Duration {
	delay := w.cfg.MinBackoff
	for i := 0; i < attempt && delay < w.cfg.MaxBackoff; i++ {
		delay *= 2
	}
	if delay > w.cfg.MaxBackoff {
		delay = w.cfg.MaxBackoff
	}

	// Full jitter over [delay/2, delay].
	half := int64(delay / 2)
	if half <= 0 {
		return delay
	}
	n, err := rand.Int(rand.Reader, big.NewInt(half))
	if err != nil {
		return delay
	}
	return time.Duration(half + n.Int64())
}
