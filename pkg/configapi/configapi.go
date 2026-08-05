// Package configapi provides public, versioned types for gateway configuration.
package configapi

import "time"

// Version is the current public configuration API version.
const Version = "v1"

// Config is the stable external configuration representation.
type Config struct {
	Schema   string       `yaml:"schema" json:"schema"`
	Server   ServerConfig `yaml:"server" json:"server"`
	PHP      PHPConfig    `yaml:"php" json:"php"`
	Routes   []Route      `yaml:"routes" json:"routes"`
	Security Security     `yaml:"security" json:"security"`
}

// ServerConfig defines gateway listener properties.
type ServerConfig struct {
	Addr              string        `yaml:"addr" json:"addr"`
	ReadTimeout       time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout" json:"write_timeout"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout" json:"read_header_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes" json:"max_header_bytes"`
}

// PHPConfig defines FastCGI/PHP backend properties.
type PHPConfig struct {
	Binary           string        `yaml:"binary" json:"binary"`
	Socket           string        `yaml:"socket" json:"socket"`
	MaxChildren      int           `yaml:"max_children" json:"max_children"`
	RequestTimeout   time.Duration `yaml:"request_timeout" json:"request_timeout"`
	ExtensionProfile string        `yaml:"extension_profile" json:"extension_profile"`
}

// Route defines routing rules.
type Route struct {
	Host       string   `yaml:"host,omitempty" json:"host,omitempty"`
	Path       string   `yaml:"path,omitempty" json:"path,omitempty"`
	PathPrefix string   `yaml:"path_prefix,omitempty" json:"path_prefix,omitempty"`
	Regex      string   `yaml:"regex,omitempty" json:"regex,omitempty"`
	Target     string   `yaml:"target,omitempty" json:"target,omitempty"`
	Status     int      `yaml:"status,omitempty" json:"status,omitempty"`
	Methods    []string `yaml:"methods,omitempty" json:"methods,omitempty"`
}

// Security defines security and rate limiting settings.
type Security struct {
	Mode              string   `yaml:"mode" json:"mode"`
	SymlinkMode       string   `yaml:"symlink_mode" json:"symlink_mode"`
	MaxBodySize       string   `yaml:"max_body_size" json:"max_body_size"`
	ProtectedPatterns []string `yaml:"protected_patterns" json:"protected_patterns"`
}
