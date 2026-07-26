package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// WritablePaths manages shared writable directories for PHP applications.
type WritablePaths struct {
	root    string
	paths   map[string]WDir
	mu      sync.RWMutex
}

// WDir represents a writable directory configuration.
type WDir struct {
	Path       string `json:"path"`
	Owner      string `json:"owner"`      // PHP user that should own the dir
	Permission string `json:"permission"` // e.g. "0775"
	Recursive  bool   `json:"recursive"`
}

// NewWritablePaths creates a writable path manager rooted at the given directory.
func NewWritablePaths(root string) *WritablePaths {
	return &WritablePaths{
		root:  root,
		paths: make(map[string]WDir),
	}
}

// Add registers a directory as writable.
func (wp *WritablePaths) Add(name, relPath, owner, perm string, recursive bool) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	wp.paths[name] = WDir{
		Path:       relPath,
		Owner:      owner,
		Permission: perm,
		Recursive:  recursive,
	}
}

// DefaultWritablePaths creates the standard writable directories for PHP apps.
func DefaultWritablePaths(root string) *WritablePaths {
	wp := NewWritablePaths(root)

	wp.Add("cache", "cache", "www-data", "0775", true)
	wp.Add("logs", "logs", "www-data", "0775", true)
	wp.Add("storage", "storage", "www-data", "0775", true)
	wp.Add("tmp", "tmp", "www-data", "0755", false)
	wp.Add("sessions", "tmp/sessions", "www-data", "0733", false)
	wp.Add("uploads", "tmp/uploads", "www-data", "0755", false)

	return wp
}

// Ensure creates all registered directories with proper permissions.
func (wp *WritablePaths) Ensure() error {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	for name, dir := range wp.paths {
		fullPath := filepath.Join(wp.root, dir.Path)

		// Create directory.
		perm := parsePerm(dir.Permission)
		if err := os.MkdirAll(fullPath, perm); err != nil {
			return fmt.Errorf("create %s (%s): %w", name, fullPath, err)
		}

		// Apply permissions.
		if err := os.Chmod(fullPath, perm); err != nil {
			return fmt.Errorf("chmod %s: %w", fullPath, err)
		}
	}

	return nil
}

// Validate checks that all registered paths exist and are writable.
func (wp *WritablePaths) Validate() error {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	for name, dir := range wp.paths {
		fullPath := filepath.Join(wp.root, dir.Path)

		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		if !info.IsDir() {
			return fmt.Errorf("%s: not a directory", name)
		}
	}

	return nil
}

// GetPath returns the absolute path for a named writable directory.
func (wp *WritablePaths) GetPath(name string) (string, error) {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	dir, ok := wp.paths[name]
	if !ok {
		return "", fmt.Errorf("unknown writable path: %s", name)
	}

	return filepath.Join(wp.root, dir.Path), nil
}

// Paths returns all registered writable paths.
func (wp *WritablePaths) Paths() map[string]WDir {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	result := make(map[string]WDir, len(wp.paths))
	for k, v := range wp.paths {
		result[k] = v
	}
	return result
}

func parsePerm(s string) os.FileMode {
	var perm os.FileMode
	for _, c := range s {
		if c >= '0' && c <= '7' {
			perm = perm*8 + os.FileMode(c-'0')
		}
	}
	return perm
}
