package config

import (
	"sync/atomic"
	"testing"
)

func TestReloaderSwap(t *testing.T) {
	initial := DefaultConfig()
	reloader := NewReloader(initial, nil)

	if reloader.Version() != 0 {
		t.Errorf("initial version = %d, want 0", reloader.Version())
	}

	newCfg := DefaultConfig()
	newCfg.Server.Addr = ":9090"

	if err := reloader.Reload(newCfg); err != nil {
		t.Fatal(err)
	}

	if reloader.Version() != 1 {
		t.Errorf("version after reload = %d, want 1", reloader.Version())
	}

	if reloader.Current().Config.Server.Addr != ":9090" {
		t.Errorf("addr = %q, want %q", reloader.Current().Config.Server.Addr, ":9090")
	}
}

func TestReloaderRejectsInvalid(t *testing.T) {
	initial := DefaultConfig()
	reloader := NewReloader(initial, nil)

	badCfg := DefaultConfig()
	badCfg.Schema = ""
	badCfg.Server.Addr = "invalid"

	err := reloader.Reload(badCfg)
	if err == nil {
		t.Error("expected error for invalid config")
	}

	if reloader.Version() != 0 {
		t.Errorf("version = %d, should still be 0", reloader.Version())
	}
}

func TestReloaderCallback(t *testing.T) {
	initial := DefaultConfig()
	var called atomic.Bool
	reloader := NewReloader(initial, func(cfg *Config) error {
		called.Store(true)
		return nil
	})

	newCfg := DefaultConfig()
	if err := reloader.Reload(newCfg); err != nil {
		t.Fatal(err)
	}

	if !called.Load() {
		t.Error("reload callback was not called")
	}
}

func TestReloadWithDrain(t *testing.T) {
	initial := DefaultConfig()
	reloader := NewReloader(initial, nil)

	var drained atomic.Bool
	d := &mockDrainer{drained: &drained}

	newCfg := DefaultConfig()
	if err := ReloadWithDrain(reloader, newCfg, d); err != nil {
		t.Fatal(err)
	}

	if !drained.Load() {
		t.Error("drain was not called")
	}
}

type mockDrainer struct {
	drained *atomic.Bool
}

func (m *mockDrainer) Drain() error {
	m.drained.Store(true)
	return nil
}
