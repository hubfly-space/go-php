package config

import (
	"fmt"
	"net"
	"time"
)

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
