package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Registry manages installed PHP runtimes.
type Registry struct {
	Root    string // e.g. ~/.gateway/runtimes
	Current string // path to "current" symlink
}

// NewRegistry creates a registry at the given root.
func NewRegistry(root string) *Registry {
	return &Registry{
		Root:    root,
		Current: filepath.Join(root, "current"),
	}
}

// Init creates the registry directory structure.
func (r *Registry) Init() error {
	dirs := []string{r.Root, filepath.Join(r.Root, "current")}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}
	return nil
}

// Install adds a runtime to the registry from a local directory.
func (r *Registry) Install(manifest *Manifest, srcDir string) (*Runtime, error) {
	id := GenerateID(
		manifest.Version,
		manifest.Platform,
		manifest.Arch,
		manifest.Flavor,
		nil, // extensions resolved later
	)

	destDir := filepath.Join(r.Root, string(id))

	// Check if already installed.
	if _, err := os.Stat(destDir); err == nil {
		return nil, fmt.Errorf("runtime %s already installed", id)
	}

	// Copy source to destination.
	if err := copyDir(srcDir, destDir); err != nil {
		return nil, fmt.Errorf("install runtime: %w", err)
	}

	// Write manifest into runtime dir.
	if err := SaveManifest(filepath.Join(destDir, "manifest.json"), manifest); err != nil {
		return nil, fmt.Errorf("save manifest: %w", err)
	}

	rt := &Runtime{
		ID:          id,
		Version:     manifest.Version,
		Platform:    manifest.Platform,
		Arch:        manifest.Arch,
		BuildFlavor: manifest.Flavor,
	}

	return rt, nil
}

// List returns all installed runtimes.
func (r *Registry) List() ([]Runtime, error) {
	entries, err := os.ReadDir(r.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list runtimes: %w", err)
	}

	var runtimes []Runtime
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == "current" {
			continue
		}

		manifestPath := filepath.Join(r.Root, e.Name(), "manifest.json")
		m, err := LoadManifest(manifestPath)
		if err != nil {
			continue // skip invalid dirs
		}

		rt := Runtime{
			ID:          RuntimeID(e.Name()),
			Version:     m.Version,
			Platform:    m.Platform,
			Arch:        m.Arch,
			BuildFlavor: m.Flavor,
		}
		runtimes = append(runtimes, rt)
	}

	return runtimes, nil
}

// Use sets a runtime as the active one.
func (r *Registry) Use(id RuntimeID) error {
	target := filepath.Join(r.Root, string(id))
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("runtime %s not found", id)
	}

	// Remove existing current symlink.
	os.Remove(r.Current)

	// Create symlink.
	if err := os.Symlink(target, r.Current); err != nil {
		return fmt.Errorf("set current: %w", err)
	}

	return nil
}

// Remove deletes a runtime from the registry.
func (r *Registry) Remove(id RuntimeID) error {
	target := filepath.Join(r.Root, string(id))
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("runtime %s not found", id)
	}

	// Check if it's the current runtime.
	if link, err := os.Readlink(r.Current); err == nil {
		if link == target {
			return fmt.Errorf("cannot remove active runtime; use another first")
		}
	}

	return os.RemoveAll(target)
}

// CurrentRuntime returns the currently active runtime, or nil.
func (r *Registry) CurrentRuntime() *Runtime {
	target, err := os.Readlink(r.Current)
	if err != nil {
		return nil
	}

	manifestPath := filepath.Join(target, "manifest.json")
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return nil
	}

	return &Runtime{
		ID:          RuntimeID(filepath.Base(target)),
		Version:     m.Version,
		Platform:    m.Platform,
		Arch:        m.Arch,
		BuildFlavor: m.Flavor,
	}
}

// RuntimeDir returns the path to a runtime's directory.
func (r *Registry) RuntimeDir(id RuntimeID) string {
	return filepath.Join(r.Root, string(id))
}

// State represents the registry state file.
type State struct {
	Current   string    `json:"current"`
	Installed []string  `json:"installed"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveState writes the registry state.
func (r *Registry) SaveState() error {
	runtimes, err := r.List()
	if err != nil {
		return err
	}

	ids := make([]string, len(runtimes))
	for i, rt := range runtimes {
		ids[i] = string(rt.ID)
	}

	current := ""
	if link, err := os.Readlink(r.Current); err == nil {
		current = filepath.Base(link)
	}

	state := State{
		Current:   current,
		Installed: ids,
		UpdatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(r.Root, "state.json"), data, 0644)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		// Skip symlinks.
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if err := os.WriteFile(target, data, info.Mode()); err != nil {
			return err
		}

		return nil
	})
}

// FindByVersion finds a runtime matching a version constraint.
func (r *Registry) FindByVersion(constraint string) (*Runtime, error) {
	runtimes, err := r.List()
	if err != nil {
		return nil, err
	}

	for i := range runtimes {
		if matchVersion(runtimes[i].Version, constraint) {
			return &runtimes[i], nil
		}
	}

	return nil, fmt.Errorf("no runtime matching %q", constraint)
}

// matchVersion checks if version matches a simple constraint.
func matchVersion(version, constraint string) bool {
	if constraint == "" || constraint == "*" {
		return true
	}

	// Exact match.
	if version == constraint {
		return true
	}

	// Prefix match (e.g. "8.3" matches "8.3.6").
	if strings.HasPrefix(version, constraint) {
		return true
	}

	return false
}
