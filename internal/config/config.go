package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level gateway configuration.
type Config struct {
	Schema   string         `yaml:"schema"`
	Server   ServerConfig   `yaml:"server"`
	PHP      PHPConfig      `yaml:"php"`
	Routes   []RouteConfig  `yaml:"routes"`
	Logging  LoggingConfig  `yaml:"logging"`
	Security SecurityConfig `yaml:"security"`
}

// ServerConfig defines HTTP server settings.
type ServerConfig struct {
	Addr              string        `yaml:"addr"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes"`
}

// PHPConfig defines PHP-FPM backend settings.
type PHPConfig struct {
	Binary         string        `yaml:"binary"`
	SocketPath     string        `yaml:"socket_path"`
	MaxChildren    int           `yaml:"max_children"`
	StartServers   int           `yaml:"start_servers"`
	MinSpare       int           `yaml:"min_spare"`
	MaxSpare       int           `yaml:"max_spare"`
	MaxRequests    int           `yaml:"max_requests"`
	RequestTimeout time.Duration `yaml:"request_timeout"`
}

// RouteConfig defines a single route.
type RouteConfig struct {
	Host       string            `yaml:"host"`
	Path       string            `yaml:"path"`
	PathPrefix string            `yaml:"path_prefix"`
	Regex      string            `yaml:"regex"`
	Target     string            `yaml:"target"`
	Status     int               `yaml:"status"`
	Methods    []string          `yaml:"methods"`
	Headers    map[string]string `yaml:"headers"`
}

// LoggingConfig defines access log settings.
type LoggingConfig struct {
	Format string `yaml:"format"` // "json", "text"
	Level  string `yaml:"level"`
}

// SecurityConfig defines security settings.
type SecurityConfig struct {
	ProtectedPatterns []string `yaml:"protected_patterns"`
	SymlinkMode       string   `yaml:"symlink_mode"` // "deny", "within_root"
	MaxBodySize       string   `yaml:"max_body_size"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Schema: "gateway/v1",
		Server: ServerConfig{
			Addr:              ":8080",
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20,
		},
		PHP: PHPConfig{
			MaxChildren:    20,
			StartServers:   2,
			MinSpare:       2,
			MaxSpare:       6,
			MaxRequests:    500,
			RequestTimeout: 60 * time.Second,
		},
		Logging: LoggingConfig{
			Format: "json",
			Level:  "info",
		},
		Security: SecurityConfig{
			SymlinkMode: "within_root",
			MaxBodySize: "20MB",
		},
	}
}

// Load reads a YAML config file and applies defaults.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
