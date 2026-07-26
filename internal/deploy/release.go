package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ReleaseState represents the lifecycle state of a release.
type ReleaseState string

const (
	StateCreated  ReleaseState = "created"
	StateActive   ReleaseState = "active"
	StateDraining ReleaseState = "draining"
	StateStopped  ReleaseState = "stopped"
	StateFailed   ReleaseState = "failed"
)

// Release represents an immutable deployment.
type Release struct {
	ID            string            `json:"id"`
	Version       string            `json:"version"`
	RuntimeID     string            `json:"runtime_id"`
	State         ReleaseState      `json:"state"`
	CreatedAt     time.Time         `json:"created_at"`
	ActivatedAt   *time.Time        `json:"activated_at,omitempty"`
	DeactivatedAt *time.Time        `json:"deactivated_at,omitempty"`
	Dir           string            `json:"dir"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// ReleaseManager manages immutable releases for a project.
type ReleaseManager struct {
	mu          sync.RWMutex
	releasesDir string
	activeID    string
	releases    map[string]*Release
}

// NewReleaseManager creates a release manager.
func NewReleaseManager(releasesDir string) *ReleaseManager {
	return &ReleaseManager{
		releasesDir: releasesDir,
		releases:    make(map[string]*Release),
	}
}

// Init creates the releases directory structure.
func (rm *ReleaseManager) Init() error {
	dirs := []string{
		rm.releasesDir,
		filepath.Join(rm.releasesDir, "active"),
		filepath.Join(rm.releasesDir, "archive"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}

	// Load existing releases.
	rm.loadReleases()
	return nil
}

// Create creates a new immutable release from a source directory.
func (rm *ReleaseManager) Create(version, runtimeID string, srcDir string, metadata map[string]string) (*Release, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	id := fmt.Sprintf("rel_%s_%d", version, time.Now().UnixNano())
	relDir := filepath.Join(rm.releasesDir, "archive", id)

	// Copy source to release directory.
	if err := copyReleaseDir(srcDir, relDir); err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	now := time.Now()
	rel := &Release{
		ID:        id,
		Version:   version,
		RuntimeID: runtimeID,
		State:     StateCreated,
		CreatedAt: now,
		Dir:       relDir,
		Metadata:  metadata,
	}

	// Save release metadata.
	if err := rm.saveRelease(rel); err != nil {
		os.RemoveAll(relDir)
		return nil, err
	}

	rm.releases[id] = rel
	return rel, nil
}

// Activate makes a release the active one (atomic swap).
func (rm *ReleaseManager) Activate(id string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rel, ok := rm.releases[id]
	if !ok {
		return fmt.Errorf("release %s not found", id)
	}

	if rel.State == StateActive {
		return fmt.Errorf("release %s is already active", id)
	}

	// Deactivate current.
	if rm.activeID != "" {
		if old, ok := rm.releases[rm.activeID]; ok {
			now := time.Now()
			old.State = StateStopped
			old.DeactivatedAt = &now
			rm.saveRelease(old)
		}
	}

	// Activate new.
	now := time.Now()
	rel.State = StateActive
	rel.ActivatedAt = &now
	rel.DeactivatedAt = nil

	// Atomic symlink swap.
	activeLink := filepath.Join(rm.releasesDir, "active")
	tmpLink := activeLink + ".tmp"

	// Remove any existing symlink and tmp.
	os.Remove(tmpLink)
	os.Remove(activeLink)

	if err := os.Symlink(rel.Dir, tmpLink); err != nil {
		rel.State = StateFailed
		return fmt.Errorf("create symlink: %w", err)
	}

	if err := os.Rename(tmpLink, activeLink); err != nil {
		os.Remove(tmpLink)
		rel.State = StateFailed
		return fmt.Errorf("activate symlink: %w", err)
	}

	rm.activeID = id
	rm.saveRelease(rel)
	return nil
}

// Deactivate stops the currently active release.
func (rm *ReleaseManager) Deactivate() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.activeID == "" {
		return nil
	}

	rel, ok := rm.releases[rm.activeID]
	if !ok {
		return nil
	}

	now := time.Now()
	rel.State = StateStopped
	rel.DeactivatedAt = &now
	rm.activeID = ""

	// Remove active symlink.
	os.Remove(filepath.Join(rm.releasesDir, "active"))

	rm.saveRelease(rel)
	return nil
}

// Rollback reverts to the previous release.
func (rm *ReleaseManager) Rollback() (*Release, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.activeID == "" {
		return nil, fmt.Errorf("no active release to rollback from")
	}

	// Find the most recently deactivated release.
	var prev *Release
	for _, rel := range rm.releases {
		if rel.ID == rm.activeID {
			continue
		}
		if rel.DeactivatedAt != nil {
			if prev == nil || rel.DeactivatedAt.After(*prev.DeactivatedAt) {
				rel2 := *rel
				prev = &rel2
			}
		}
	}

	if prev == nil {
		return nil, fmt.Errorf("no previous release found")
	}

	// Must unlock before calling Activate.
	rm.mu.Unlock()
	if err := rm.Activate(prev.ID); err != nil {
		return nil, fmt.Errorf("rollback: %w", err)
	}
	rm.mu.Lock()

	return rm.releases[prev.ID], nil
}

// Active returns the currently active release, or nil.
func (rm *ReleaseManager) Active() *Release {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.activeID == "" {
		return nil
	}
	rel := rm.releases[rm.activeID]
	return rel
}

// List returns all releases sorted by creation time.
func (rm *ReleaseManager) List() []*Release {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]*Release, 0, len(rm.releases))
	for _, rel := range rm.releases {
		result = append(result, rel)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result
}

// Get returns a release by ID.
func (rm *ReleaseManager) Get(id string) (*Release, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	rel, ok := rm.releases[id]
	if !ok {
		return nil, fmt.Errorf("release %s not found", id)
	}
	return rel, nil
}

// Archive removes old stopped releases, keeping the last n.
func (rm *ReleaseManager) Archive(keep int) (int, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	var stopped []*Release
	for _, rel := range rm.releases {
		if rel.State == StateStopped || rel.State == StateFailed {
			stopped = append(stopped, rel)
		}
	}

	sort.Slice(stopped, func(i, j int) bool {
		return stopped[i].CreatedAt.Before(stopped[j].CreatedAt)
	})

	removed := 0
	for i := 0; i < len(stopped)-keep; i++ {
		rel := stopped[i]
		os.RemoveAll(rel.Dir)
		os.Remove(rel.Dir + ".json")
		delete(rm.releases, rel.ID)
		removed++
	}

	return removed, nil
}

func (rm *ReleaseManager) saveRelease(rel *Release) error {
	data, err := json.MarshalIndent(rel, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(rm.releasesDir, "archive", rel.ID+".json")
	return os.WriteFile(path, data, 0644)
}

func (rm *ReleaseManager) loadReleases() {
	archiveDir := filepath.Join(rm.releasesDir, "archive")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		path := filepath.Join(archiveDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var rel Release
		if err := json.Unmarshal(data, &rel); err != nil {
			continue
		}

		rm.releases[rel.ID] = &rel
		if rel.State == StateActive {
			rm.activeID = rel.ID
		}
	}
}

func copyReleaseDir(src, dst string) error {
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

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(target, data, info.Mode())
	})
}
