package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Manifest describes a PHP runtime artifact.
type Manifest struct {
	Version    string           `json:"version"`
	Platform   string           `json:"platform"`
	Arch       string           `json:"arch"`
	Flavor     string           `json:"flavor"`
	SHA256     string           `json:"sha256"`
	Size       int64            `json:"size"`
	Extensions []ExtensionEntry `json:"extensions"`
	URL        string           `json:"url"`
	Signature  string           `json:"signature,omitempty"`
}

// ExtensionEntry describes an extension in the manifest.
type ExtensionEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	URL     string `json:"url"`
}

// Index is a list of available runtimes.
type Index struct {
	Runtimes []Manifest `json:"runtimes"`
}

// LoadManifest reads a manifest file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Version == "" {
		return nil, fmt.Errorf("manifest: version is required")
	}
	if m.Platform == "" {
		return nil, fmt.Errorf("manifest: platform is required")
	}
	if m.Arch == "" {
		return nil, fmt.Errorf("manifest: arch is required")
	}
	return &m, nil
}

// SaveManifest writes a manifest file atomically.
func SaveManifest(path string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return os.Rename(tmp, path)
}

// LoadIndex reads an index file.
func LoadIndex(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	return &idx, nil
}

// FindManifest looks for manifest.json in a runtime directory.
func FindManifest(runtimeDir string) (*Manifest, string, error) {
	path := filepath.Join(runtimeDir, "manifest.json")
	m, err := LoadManifest(path)
	if err != nil {
		return nil, "", err
	}
	return m, path, nil
}
