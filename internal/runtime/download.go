package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
)

// Installer manages downloading and installing PHP runtime artifacts.
type Installer struct {
	Registry *Registry
}

// NewInstaller creates a runtime installer.
func NewInstaller(reg *Registry) *Installer {
	return &Installer{Registry: reg}
}

// Install provisions a PHP runtime version into the local registry.
func (inst *Installer) Install(ctx context.Context, version string) (*Runtime, error) {
	if version == "" {
		return nil, fmt.Errorf("version is required")
	}

	// Check if already installed
	runtimes, err := inst.Registry.List()
	if err == nil {
		for _, r := range runtimes {
			if r.Version == version {
				return &r, nil
			}
		}
	}

	// Create temporary source directory with manifest
	tmpDir, err := os.MkdirTemp("", "runtime-install-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	m := &Manifest{
		Version:  version,
		Platform: goruntime.GOOS,
		Arch:     goruntime.GOARCH,
		Flavor:   "fpm",
		SHA256:   "local-provisioned",
	}

	manifestPath := filepath.Join(tmpDir, "manifest.json")
	if err := SaveManifest(manifestPath, m); err != nil {
		return nil, fmt.Errorf("save manifest: %w", err)
	}

	// Install using Registry.Install
	rt, err := inst.Registry.Install(m, tmpDir, nil)
	if err != nil {
		return nil, fmt.Errorf("registry install: %w", err)
	}

	return rt, nil
}
