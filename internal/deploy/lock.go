package deploy

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// LockFile records the exact runtime state for reproducible deploys.
type LockFile struct {
	Schema         string          `json:"schema"`
	PHPVersion     string          `json:"php_version"`
	RuntimeID      string          `json:"runtime_id"`
	ManifestHash   string          `json:"manifest_hash"`
	Extensions     []LockExtension `json:"extensions"`
	Profile        string          `json:"profile,omitempty"`
	GeneratedAt    time.Time       `json:"generated_at"`
	GatewayVersion string          `json:"gateway_version"`
	Checksum       string          `json:"checksum"`
}

// LockExtension records a locked extension.
type LockExtension struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Enabled bool   `json:"enabled"`
}

// LoadLockFile reads a lock file.
func LoadLockFile(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read lock file: %w", err)
	}
	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse lock file: %w", err)
	}
	if lf.Schema == "" {
		return nil, fmt.Errorf("lock file: schema is required")
	}
	return &lf, nil
}

// SaveLockFile writes a lock file atomically.
func SaveLockFile(path string, lf *LockFile) error {
	lf.GeneratedAt = time.Now()

	// Compute checksum.
	lf.Checksum = ""
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lock file: %w", err)
	}
	h := sha256.Sum256(data)
	lf.Checksum = fmt.Sprintf("%x", h)

	// Re-marshal with checksum.
	data, err = json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lock file: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	return os.Rename(tmp, path)
}

// Verify checks the lock file checksum.
func (lf *LockFile) Verify() error {
	saved := lf.Checksum
	lf.Checksum = ""
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}
	h := sha256.Sum256(data)
	lf.Checksum = saved

	if fmt.Sprintf("%x", h) != saved {
		return fmt.Errorf("lock file checksum mismatch")
	}
	return nil
}

// NewLockFile creates a lock file from a runtime and extensions.
func NewLockFile(phpVersion, runtimeID, manifestHash, profile, gwVersion string, extensions []LockExtension) *LockFile {
	return &LockFile{
		Schema:         "gateway-lock/v1",
		PHPVersion:     phpVersion,
		RuntimeID:      runtimeID,
		ManifestHash:   manifestHash,
		Extensions:     extensions,
		Profile:        profile,
		GatewayVersion: gwVersion,
	}
}
