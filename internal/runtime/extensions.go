package runtime

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// ListInstalled returns all installed extensions from the manifest.
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

// Enable creates a conf.d file for an extension (extension type).
func (m *ExtensionManager) Enable(name string) error {
	return m.EnableWithType(name, "extension")
}

// EnableWithType creates a conf.d file for an extension with the given type.
// Supported types: "extension", "zend_extension".
func (m *ExtensionManager) EnableWithType(name, extType string) error {
	confDir := filepath.Join(m.RuntimeDir, "conf.d")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return fmt.Errorf("create conf.d: %w", err)
	}

	var content string
	switch extType {
	case "zend_extension":
		content = fmt.Sprintf("zend_extension=%s.so\n", name)
	default:
		content = fmt.Sprintf("extension=%s.so\n", name)
	}

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

// BulkEnable enables multiple extensions at once.
func (m *ExtensionManager) BulkEnable(names []string) error {
	for _, name := range names {
		if err := m.Enable(name); err != nil {
			return fmt.Errorf("bulk enable %s: %w", name, err)
		}
	}
	return nil
}

// BulkDisable disables multiple extensions at once.
func (m *ExtensionManager) BulkDisable(names []string) error {
	for _, name := range names {
		if err := m.Disable(name); err != nil {
			return fmt.Errorf("bulk disable %s: %w", name, err)
		}
	}
	return nil
}

// GetEnabled returns the list of currently enabled extension names.
func (m *ExtensionManager) GetEnabled() ([]string, error) {
	confDir := filepath.Join(m.RuntimeDir, "conf.d")
	entries, err := os.ReadDir(confDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Expect format: 20-{name}.ini
		if strings.HasSuffix(name, ".ini") && len(name) > 7 {
			extName := name[3 : len(name)-4] // strip "20-" prefix and ".ini" suffix
			if extName != "" {
				names = append(names, extName)
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

// ValidateExists checks if an extension's .so file exists in the runtime.
func (m *ExtensionManager) ValidateExists(name string) bool {
	// Check common locations.
	locations := []string{
		filepath.Join(m.RuntimeDir, "lib", "php", "extensions", name+".so"),
		filepath.Join(m.RuntimeDir, "extensions", name+".so"),
		filepath.Join(m.RuntimeDir, "lib", name+".so"),
	}
	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return true
		}
	}
	return false
}

// ApplyExtensions applies a resolved set of extensions atomically.
// It ensures only the specified extensions are enabled (disables others).
func (m *ExtensionManager) ApplyExtensions(extensions []configResolvedExtension) error {
	// Get current enabled set.
	current, err := m.GetEnabled()
	if err != nil {
		return fmt.Errorf("get enabled: %w", err)
	}

	// Build target set.
	target := make(map[string]string)
	for _, ext := range extensions {
		target[ext.Name] = ext.Type
	}

	// Disable extensions not in target set.
	for _, name := range current {
		if _, ok := target[name]; !ok {
			if err := m.Disable(name); err != nil {
				return fmt.Errorf("disable %s: %w", name, err)
			}
		}
	}

	// Enable extensions in target set.
	for _, ext := range extensions {
		if !m.IsEnabled(ext.Name) || m.getEnabledType(ext.Name) != ext.Type {
			if err := m.EnableWithType(ext.Name, ext.Type); err != nil {
				return fmt.Errorf("enable %s: %w", ext.Name, err)
			}
		}
	}

	return nil
}

// configResolvedExtension mirrors config.ResolvedExtension without import cycle.
type configResolvedExtension struct {
	Name string
	Type string
}

// getEnabledType reads the conf.d file to determine the extension type.
func (m *ExtensionManager) getEnabledType(name string) string {
	path := filepath.Join(m.RuntimeDir, "conf.d", fmt.Sprintf("20-%s.ini", name))
	data, err := os.ReadFile(path)
	if err != nil {
		return "extension"
	}
	if strings.HasPrefix(string(data), "zend_extension") {
		return "zend_extension"
	}
	return "extension"
}

// ExtensionProfile is a predefined set of extensions.
type ExtensionProfile struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Extensions  []string `json:"extensions"`
}

// BuiltInProfiles returns the standard extension profiles.
// Only lists loadable extensions (built-in PHP modules like core, standard, date, json are excluded).
func BuiltInProfiles() []ExtensionProfile {
	return []ExtensionProfile{
		{
			Name:        "minimal",
			Description: "Bare minimum for basic PHP scripts",
			Extensions:  []string{"pdo", "pdo_sqlite", "mbstring", "curl", "json", "xml"},
		},
		{
			Name:        "web-standard",
			Description: "Standard web application extensions",
			Extensions: []string{
				"pdo", "pdo_mysql", "pdo_sqlite",
				"mysqli", "mbstring", "curl", "gd",
				"bcmath", "fileinfo", "intl", "zip", "exif",
			},
		},
		{
			Name:        "wordpress",
			Description: "WordPress-optimized extension set",
			Extensions: []string{
				"pdo", "pdo_mysql", "mysqli",
				"mbstring", "curl", "gd",
				"bcmath", "fileinfo", "intl", "zip", "exif",
				"sockets", "openssl",
			},
		},
		{
			Name:        "laravel",
			Description: "Laravel-optimized extension set",
			Extensions: []string{
				"pdo", "pdo_mysql", "pdo_sqlite",
				"pdo_pgsql", "pgsql", "mysqli", "mbstring", "curl", "gd",
				"bcmath", "fileinfo",
				"intl", "zip", "exif", "openssl", "sodium",
			},
		},
		{
			Name:        "development",
			Description: "Extensions useful in development (includes xdebug)",
			Extensions: []string{
				"pdo", "pdo_mysql", "pdo_sqlite",
				"mysqli", "mbstring", "curl", "gd",
				"bcmath", "fileinfo", "intl", "zip", "exif",
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
