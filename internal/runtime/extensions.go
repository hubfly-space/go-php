package runtime

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ExtensionManager handles PHP extension installation and configuration.
type ExtensionManager struct {
	RuntimeDir string
}

// NewExtensionManager creates an extension manager for a runtime.
func NewExtensionManager(runtimeDir string) *ExtensionManager {
	return &ExtensionManager{RuntimeDir: runtimeDir}
}

// InstalledExtension represents an installed extension.
type InstalledExtension struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Enabled bool   `json:"enabled"`
}

// ListInstalled returns all installed extensions.
func (m *ExtensionManager) ListInstalled() ([]InstalledExtension, error) {
	manifestPath := filepath.Join(m.RuntimeDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var manifest struct {
		Extensions []InstalledExtension `json:"extensions"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return manifest.Extensions, nil
}

// Enable creates a conf.d file for an extension.
func (m *ExtensionManager) Enable(name string) error {
	confDir := filepath.Join(m.RuntimeDir, "conf.d")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return fmt.Errorf("create conf.d: %w", err)
	}

	content := fmt.Sprintf("extension=%s.so\n", name)
	path := filepath.Join(confDir, fmt.Sprintf("20-%s.ini", name))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("enable %s: %w", name, err)
	}
	return nil
}

// Disable removes the conf.d file for an extension.
func (m *ExtensionManager) Disable(name string) error {
	path := filepath.Join(m.RuntimeDir, "conf.d", fmt.Sprintf("20-%s.ini", name))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("disable %s: %w", name, err)
	}
	return nil
}

// IsEnabled checks if an extension is enabled.
func (m *ExtensionManager) IsEnabled(name string) bool {
	path := filepath.Join(m.RuntimeDir, "conf.d", fmt.Sprintf("20-%s.ini", name))
	_, err := os.Stat(path)
	return err == nil
}

// ExtensionProfile is a predefined set of extensions.
type ExtensionProfile struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Extensions  []string `json:"extensions"`
}

// BuiltInProfiles returns the standard extension profiles.
func BuiltInProfiles() []ExtensionProfile {
	return []ExtensionProfile{
		{
			Name:        "minimal",
			Description: "Bare minimum for basic PHP scripts",
			Extensions:  []string{"core", "standard", "date"},
		},
		{
			Name:        "web-standard",
			Description: "Standard web application extensions",
			Extensions:  []string{
				"core", "standard", "date", "pdo", "pdo_mysql", "pdo_sqlite",
				"mysqli", "mbstring", "curl", "gd", "json", "xml", "xmlwriter",
				"tokenizer", "bcmath", "fileinfo", "intl", "zip", "exif",
			},
		},
		{
			Name:        "wordpress",
			Description: "WordPress-optimized extension set",
			Extensions: []string{
				"core", "standard", "date", "pdo", "pdo_mysql", "mysqli",
				"mbstring", "curl", "gd", "json", "xml", "xmlwriter",
				"tokenizer", "bcmath", "fileinfo", "intl", "zip", "exif",
				"sockets", "openssl",
			},
		},
		{
			Name:        "laravel",
			Description: "Laravel-optimized extension set",
			Extensions: []string{
				"core", "standard", "date", "pdo", "pdo_mysql", "pdo_sqlite",
				"pdo_pgsql", "pgsql", "mysqli", "mbstring", "curl", "gd",
				"json", "xml", "xmlwriter", "tokenizer", "bcmath", "fileinfo",
				"intl", "zip", "exif", "openssl", "sodium", "tokenizer",
			},
		},
		{
			Name:        "development",
			Description: "Extensions useful in development (includes xdebug)",
			Extensions: []string{
				"core", "standard", "date", "pdo", "pdo_mysql", "pdo_sqlite",
				"mysqli", "mbstring", "curl", "gd", "json", "xml", "xmlwriter",
				"tokenizer", "bcmath", "fileinfo", "intl", "zip", "exif",
				"xdebug", "pcov",
			},
		},
	}
}

// ProfileByName returns a profile by name, or nil if not found.
func ProfileByName(name string) *ExtensionProfile {
	for _, p := range BuiltInProfiles() {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

// ProfileExtensions returns sorted extension names from a profile.
func ProfileExtensions(profileName string) ([]string, error) {
	p := ProfileByName(profileName)
	if p == nil {
		available := make([]string, 0)
		for _, pp := range BuiltInProfiles() {
			available = append(available, pp.Name)
		}
		return nil, fmt.Errorf("unknown profile %q; available: %v", profileName, available)
	}
	exts := make([]string, len(p.Extensions))
	copy(exts, p.Extensions)
	sort.Strings(exts)
	return exts, nil
}

// ComputeExtensionHash computes a deterministic hash of extension set.
func ComputeExtensionHash(extensions []string) string {
	sorted := make([]string, len(extensions))
	copy(sorted, extensions)
	sort.Strings(sorted)

	h := sha256.New()
	for _, e := range sorted {
		h.Write([]byte(e))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil)[:8])
}
