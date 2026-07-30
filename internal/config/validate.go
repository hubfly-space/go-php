package config

import (
	"fmt"
	"net"
	"time"
)

// knownExtensionProfiles are the valid extension profile names.
var knownExtensionProfiles = map[string]bool{
	"minimal":       true,
	"web-standard":  true,
	"wordpress":     true,
	"laravel":       true,
	"development":   true,
}

// Validate checks a config for semantic errors.
func Validate(cfg *Config) error {
	if cfg.Schema == "" {
		return fmt.Errorf("schema field is required")
	}

	// Validate server.
	if cfg.Server.Addr == "" {
		return fmt.Errorf("server.addr is required")
	}
	host, port, err := net.SplitHostPort(cfg.Server.Addr)
	if err != nil {
		return fmt.Errorf("server.addr: %w", err)
	}
	if host != "" {
		if ip := net.ParseIP(host); ip == nil {
			return fmt.Errorf("server.addr: %q is not a valid IP", host)
		}
	}
	if port == "" {
		return fmt.Errorf("server.addr: port is required")
	}

	// Validate PHP.
	if cfg.PHP.MaxChildren <= 0 {
		cfg.PHP.MaxChildren = 20
	}
	if cfg.PHP.StartServers <= 0 {
		cfg.PHP.StartServers = 2
	}
	if cfg.PHP.RequestTimeout <= 0 {
		cfg.PHP.RequestTimeout = 60 * time.Second
	}

	// Validate extension config.
	if cfg.PHP.ExtensionProfile != "" && len(cfg.PHP.Extensions) > 0 {
		return fmt.Errorf("php: cannot specify both extension_profile and individual extensions; choose one")
	}
	if cfg.PHP.ExtensionProfile != "" && !knownExtensionProfiles[cfg.PHP.ExtensionProfile] {
		return fmt.Errorf("php: unknown extension_profile %q; valid: minimal, web-standard, wordpress, laravel, development", cfg.PHP.ExtensionProfile)
	}
	for _, ext := range cfg.PHP.Extensions {
		if ext.Name == "" {
			return fmt.Errorf("php: extension name is required")
		}
		if ext.Type != "" && ext.Type != "extension" && ext.Type != "zend_extension" {
			return fmt.Errorf("php: extension %q: type must be 'extension' or 'zend_extension'", ext.Name)
		}
	}
	seenExt := make(map[string]bool)
	for _, ext := range cfg.PHP.Extensions {
		if seenExt[ext.Name] {
			return fmt.Errorf("php: duplicate extension %q", ext.Name)
		}
		seenExt[ext.Name] = true
	}

	// Validate per-route extension overrides.
	for i, route := range cfg.Routes {
		if route.ExtensionOverride != nil {
			for _, name := range route.ExtensionOverride.Enable {
				if name == "" {
					return fmt.Errorf("routes[%d]: extension enable list contains empty name", i)
				}
			}
			for _, name := range route.ExtensionOverride.Disable {
				if name == "" {
					return fmt.Errorf("routes[%d]: extension disable list contains empty name", i)
				}
			}
			// Check no overlap between enable/disable.
			enableSet := make(map[string]bool, len(route.ExtensionOverride.Enable))
			for _, name := range route.ExtensionOverride.Enable {
				enableSet[name] = true
			}
			for _, name := range route.ExtensionOverride.Disable {
				if enableSet[name] {
					return fmt.Errorf("routes[%d]: extension %q is in both enable and disable lists", i, name)
				}
			}
		}
	}

	// Validate logging.
	switch cfg.Logging.Format {
	case "json", "text":
		// ok
	case "":
		cfg.Logging.Format = "json"
	default:
		return fmt.Errorf("logging.format must be 'json' or 'text', got %q", cfg.Logging.Format)
	}

	// Validate security.
	switch cfg.Security.SymlinkMode {
	case "deny", "within_root":
		// ok
	case "":
		cfg.Security.SymlinkMode = "within_root"
	default:
		return fmt.Errorf("security.symlink_mode must be 'deny' or 'within_root', got %q", cfg.Security.SymlinkMode)
	}

	return nil
}
