package config

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// knownExtensionProfiles are the valid extension profile names.
var knownExtensionProfiles = map[string]bool{
	"minimal":      true,
	"web-standard": true,
	"wordpress":    true,
	"laravel":      true,
	"development":  true,
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

	// Validate isolation.
	switch cfg.PHP.Isolation.Mode {
	case "", "none":
		cfg.PHP.Isolation.Mode = "none"
	case "process", "namespace", "cgroup":
		// ok
	default:
		return fmt.Errorf("php.isolation.mode must be one of 'none', 'process', 'namespace', 'cgroup', got %q",
			cfg.PHP.Isolation.Mode)
	}
	if cfg.PHP.Isolation.MemoryLimit != "" {
		if _, err := ParseByteSize(cfg.PHP.Isolation.MemoryLimit); err != nil {
			return fmt.Errorf("php.isolation.memory_limit: %w", err)
		}
	}
	if cfg.PHP.Isolation.PIDLimit < 0 {
		return fmt.Errorf("php.isolation.pid_limit cannot be negative")
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

	switch cfg.Security.Mode {
	case "off", "observe", "balanced", "strict":
		// ok
	case "":
		cfg.Security.Mode = "balanced"
	default:
		return fmt.Errorf("security.mode must be one of 'off', 'observe', 'balanced', 'strict', got %q", cfg.Security.Mode)
	}

	// Resolve max_body_size once, here, so the request path never parses a
	// string and a typo fails at load rather than at the first large upload.
	if cfg.Security.MaxBodySize == "" {
		cfg.Security.maxBodyBytes = 0
	} else {
		n, err := ParseByteSize(cfg.Security.MaxBodySize)
		if err != nil {
			return fmt.Errorf("security.max_body_size: %w", err)
		}
		cfg.Security.maxBodyBytes = n
	}

	if cfg.Security.RateLimit.Enabled {
		if cfg.Security.RateLimit.RequestsPerMinute <= 0 {
			return fmt.Errorf("security.rate_limit.requests_per_minute must be positive when rate limiting is enabled")
		}
		if cfg.Security.RateLimit.Burst < 0 {
			return fmt.Errorf("security.rate_limit.burst cannot be negative")
		}
		if cfg.Security.RateLimit.Burst == 0 {
			cfg.Security.RateLimit.Burst = cfg.Security.RateLimit.RequestsPerMinute
		}
	}

	// Validate TLS.
	switch cfg.TLS.Mode {
	case "", "disabled":
		cfg.TLS.Mode = "disabled"
	case "files":
		// A default cert pair or a cert directory is required; without either
		// the listener would start and then fail every handshake.
		if cfg.TLS.CertDir == "" && (cfg.TLS.CertFile == "" || cfg.TLS.KeyFile == "") {
			return fmt.Errorf("tls.mode is 'files' but neither tls.cert_dir nor both tls.cert_file and tls.key_file are set")
		}
		if (cfg.TLS.CertFile == "") != (cfg.TLS.KeyFile == "") {
			return fmt.Errorf("tls.cert_file and tls.key_file must be set together")
		}
	case "acme":
		return fmt.Errorf("tls.mode 'acme' is not implemented: the ACME client does not contact a CA " +
			"and would serve a self-signed certificate. Use tls.mode 'files' with a certificate from " +
			"your CA, or terminate TLS upstream")
	default:
		return fmt.Errorf("tls.mode must be 'disabled' or 'files', got %q", cfg.TLS.Mode)
	}

	// Validate observability.
	if cfg.Observability.Metrics.Path == "" {
		cfg.Observability.Metrics.Path = "/metrics"
	}
	if !strings.HasPrefix(cfg.Observability.Metrics.Path, "/") {
		return fmt.Errorf("observability.metrics.path must start with '/', got %q", cfg.Observability.Metrics.Path)
	}
	if cfg.Observability.Tracing.Retention <= 0 {
		// An unbounded span map is a memory leak, so a non-positive retention
		// is corrected rather than accepted.
		cfg.Observability.Tracing.Retention = 5 * time.Minute
	}

	return nil
}
