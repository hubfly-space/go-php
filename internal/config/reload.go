package config

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Snapshot is an immutable configuration snapshot.
type Snapshot struct {
	Config     *Config
	CompiledAt time.Time
	Version    uint64
}

// Reloader manages configuration reload with atomic swap.
type Reloader struct {
	current  atomic.Pointer[Snapshot]
	version  uint64
	onReload func(*Config) error // called after successful swap
}

// NewReloader creates a configuration reloader.
func NewReloader(initial *Config, onReload func(*Config) error) *Reloader {
	r := &Reloader{onReload: onReload}
	snap := &Snapshot{
		Config:     initial,
		CompiledAt: time.Now(),
		Version:    0,
	}
	r.current.Store(snap)
	return r
}

// Current returns the current configuration snapshot.
func (r *Reloader) Current() *Snapshot {
	return r.current.Load()
}

// Reload validates and activates a new configuration.
// Returns error if validation fails; never replaces known-good state.
func (r *Reloader) Reload(newCfg *Config) error {
	// Validate new config.
	if err := Validate(newCfg); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	// Build candidate snapshot.
	r.version++
	candidate := &Snapshot{
		Config:     newCfg,
		CompiledAt: time.Now(),
		Version:    r.version,
	}

	// Atomic swap.
	r.current.Store(candidate)

	// Notify callback.
	if r.onReload != nil {
		if err := r.onReload(newCfg); err != nil {
			return fmt.Errorf("reload callback: %w", err)
		}
	}

	return nil
}

// Version returns the current config version.
func (r *Reloader) Version() uint64 {
	return r.Current().Version
}

// Drainable is implemented by components that can drain old state.
type Drainable interface {
	Drain() error
}

// ReloadWithDrain validates, swaps, and drains old state.
func ReloadWithDrain(reloader *Reloader, newCfg *Config, drainables ...Drainable) error {
	if err := reloader.Reload(newCfg); err != nil {
		return err
	}

	// Drain old state (best-effort).
	for _, d := range drainables {
		d.Drain()
	}

	return nil
}
