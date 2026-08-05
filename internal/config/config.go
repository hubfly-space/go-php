package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level gateway configuration.
type Config struct {
	Schema        string              `yaml:"schema"`
	Server        ServerConfig        `yaml:"server"`
	PHP           PHPConfig           `yaml:"php"`
	Routes        []RouteConfig       `yaml:"routes"`
	Logging       LoggingConfig       `yaml:"logging"`
	Security      SecurityConfig      `yaml:"security"`
	Observability ObservabilityConfig `yaml:"observability"`
	TLS           TLSConfig           `yaml:"tls"`
	Static        StaticConfig        `yaml:"static"`
}

// StaticConfig defines static file serving behavior (§12).
type StaticConfig struct {
	// MaxAge is the default Cache-Control max-age for static files. Zero
	// disables Cache-Control entirely, which is the historical behavior.
	MaxAge time.Duration `yaml:"max_age"`

	// ImmutablePaths are prefixes whose contents are content-addressed and can
	// be cached forever.
	ImmutablePaths []string `yaml:"immutable_paths"`

	// NoCachePaths are prefixes that must never be cached.
	NoCachePaths []string `yaml:"no_cache_paths"`

	// Precompressed enables serving a sibling .br or .gz file when the client
	// accepts that encoding.
	Precompressed bool `yaml:"precompressed"`
}

// TLSConfig defines HTTPS settings (§30).
type TLSConfig struct {
	// Mode is one of "disabled", "files", or "acme".
	//
	// "acme" is rejected at config load. internal/tls/acme.go does not contact
	// an ACME server — it returns a self-signed certificate under a Let's
	// Encrypt-shaped API. Serving a self-signed cert while claiming automatic
	// TLS is worse than declining to offer the mode, so it fails loudly until a
	// real implementation lands.
	Mode string `yaml:"mode"`

	// CertFile and KeyFile are the default certificate, used when SNI does not
	// match a more specific one.
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`

	// CertDir, when set, is scanned for per-host certificate pairs for SNI.
	CertDir string `yaml:"cert_dir"`

	// RedirectFrom, when set, starts a plain HTTP listener on that address that
	// 301s to HTTPS.
	RedirectFrom string `yaml:"redirect_from"`
}

// Enabled reports whether an HTTPS listener should be started.
func (t TLSConfig) Enabled() bool { return t.Mode == "files" }

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
	Binary           string            `yaml:"binary"`
	SocketPath       string            `yaml:"socket_path"`
	MaxChildren      int               `yaml:"max_children"`
	StartServers     int               `yaml:"start_servers"`
	MinSpare         int               `yaml:"min_spare"`
	MaxSpare         int               `yaml:"max_spare"`
	MaxRequests      int               `yaml:"max_requests"`
	RequestTimeout   time.Duration     `yaml:"request_timeout"`
	ExtensionProfile string            `yaml:"extension_profile"`
	Extensions       []ExtensionConfig `yaml:"extensions"`
	PhpIni           []IniSetting      `yaml:"php_ini"`
	Isolation        IsolationConfig   `yaml:"isolation"`
}

// IsolationConfig selects the OS-level isolation tier for php-fpm (§28.1).
//
// The default is Tier 0 — no isolation. §28.1 requires the *claimed* isolation
// level to be accurate: "The project must not claim safe untrusted
// multi-tenancy at Tier 0 or Tier 1." Raising this to "namespace" or "cgroup"
// does not by itself make the gateway safe for untrusted tenants.
type IsolationConfig struct {
	// Mode is one of "none", "process", "namespace", or "cgroup".
	Mode string `yaml:"mode"`

	// User drops privileges for the php-fpm process. There is deliberately no
	// group key: the isolator resolves the group from the user, and a config
	// field that silently does nothing is worse than its absence.
	User string `yaml:"user"`

	// MemoryLimit is a cgroup v2 memory cap, e.g. "512M".
	MemoryLimit string `yaml:"memory_limit"`

	// PIDLimit caps the number of processes in the cgroup.
	PIDLimit int `yaml:"pid_limit"`
}

// ExtensionConfig defines a single PHP extension configuration.
type ExtensionConfig struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`    // "extension" or "zend_extension"
	Enabled *bool  `yaml:"enabled"` // nil means enabled
}

// IniSetting defines a php.ini directive.
type IniSetting struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// ExtensionOverride defines per-route extension overrides.
type ExtensionOverride struct {
	Enable  []string `yaml:"enable"`
	Disable []string `yaml:"disable"`
}

// RouteConfig defines a single route.
type RouteConfig struct {
	Host              string             `yaml:"host"`
	Path              string             `yaml:"path"`
	PathPrefix        string             `yaml:"path_prefix"`
	Regex             string             `yaml:"regex"`
	Target            string             `yaml:"target"`
	Status            int                `yaml:"status"`
	Methods           []string           `yaml:"methods"`
	Headers           map[string]string  `yaml:"headers"`
	ExtensionOverride *ExtensionOverride `yaml:"extensions,omitempty"`
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

	// Mode selects how policy rules are enforced (§23.4). One of "off",
	// "observe", "balanced", or "strict". Note that "off" disables *rules*
	// only — structural protections such as path canonicalization and script
	// mapping are never disabled by this setting.
	Mode string `yaml:"mode"`

	RateLimit RateLimitConfig `yaml:"rate_limit"`

	// maxBodyBytes is MaxBodySize resolved at validation time so the request
	// path never has to parse a string. Zero means unlimited.
	maxBodyBytes int64
}

// MaxBodyBytes returns the parsed value of MaxBodySize. It is populated by
// Validate; zero means no limit.
func (s SecurityConfig) MaxBodyBytes() int64 { return s.maxBodyBytes }

// RateLimitConfig defines per-client request rate limiting (§24.3).
type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
	Burst             int  `yaml:"burst"`
}

// ObservabilityConfig defines metrics, tracing, and log redaction settings.
type ObservabilityConfig struct {
	Metrics MetricsConfig `yaml:"metrics"`
	Tracing TracingConfig `yaml:"tracing"`

	// RedactKeys are log attribute keys whose values are replaced before the
	// record is written (§32.2).
	RedactKeys []string `yaml:"redact_keys"`
}

// MetricsConfig controls the Prometheus endpoint. The endpoint is served on the
// management listener, never on the public one — §5.5 requires control-plane
// and data-plane separation.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// TracingConfig controls request tracing.
type TracingConfig struct {
	Enabled bool `yaml:"enabled"`

	// Retention bounds how long finished spans are held in memory before
	// Cleanup discards them. The span map is otherwise unbounded.
	Retention time.Duration `yaml:"retention"`
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
			MaxChildren:      20,
			StartServers:     2,
			MinSpare:         2,
			MaxSpare:         6,
			MaxRequests:      500,
			RequestTimeout:   60 * time.Second,
			ExtensionProfile: "",
			Extensions:       nil,
			PhpIni:           nil,
		},
		Logging: LoggingConfig{
			Format: "json",
			Level:  "info",
		},
		Security: SecurityConfig{
			SymlinkMode: "within_root",
			MaxBodySize: "20MB",
			Mode:        "balanced",
			RateLimit: RateLimitConfig{
				Enabled:           false,
				RequestsPerMinute: 600,
				Burst:             100,
			},
		},
		Static: StaticConfig{
			MaxAge:         time.Hour,
			ImmutablePaths: []string{"/assets/", "/static/", "/dist/", "/build/"},
			Precompressed:  true,
		},
		Observability: ObservabilityConfig{
			Metrics: MetricsConfig{Enabled: true, Path: "/metrics"},
			Tracing: TracingConfig{Enabled: false, Retention: 5 * time.Minute},
			RedactKeys: []string{
				"authorization", "cookie", "set-cookie",
				"password", "token", "secret", "api_key",
			},
		},
	}
}

// Parse decodes YAML bytes into a Config struct and validates it.
func Parse(data []byte) (*Config, error) {
	cfg := DefaultConfig()

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Load reads a YAML config file and applies defaults.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return Parse(data)
}
