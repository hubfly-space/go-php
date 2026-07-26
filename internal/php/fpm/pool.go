package fpm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PoolConfig defines a PHP-FPM pool.
type PoolConfig struct {
	Name            string
	User            string
	Group           string
	SocketPath      string
	ListenMode      string // e.g. "0660"
	MaxChildren     int
	StartServers    int
	MinSpare        int
	MaxSpare        int
	MaxRequests     int
	RequestTimeout  int // seconds
	ProcessIdle     int // seconds
	StatusPath      string
	ClearEnv        bool
	SecurityExt     string // e.g. ".php"
	PhpIniPath      string
	AccessLog       string
	ErrorLog        string
	CustomDirectives map[string]string
}

// DefaultPoolConfig returns sensible defaults.
func DefaultPoolConfig(name string) *PoolConfig {
	return &PoolConfig{
		Name:           name,
		User:           "www-data",
		Group:          "www-data",
		ListenMode:     "0660",
		MaxChildren:    20,
		StartServers:   2,
		MinSpare:       1,
		MaxSpare:       6,
		MaxRequests:    500,
		RequestTimeout: 60,
		ProcessIdle:    10,
		ClearEnv:       true,
		SecurityExt:    ".php",
	}
}

// Generate produces the FPM pool configuration file content.
func (c *PoolConfig) Generate() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("[%s]\n", c.Name))

	// User/Group.
	if c.User != "" {
		b.WriteString(fmt.Sprintf("user = %s\n", c.User))
	}
	if c.Group != "" {
		b.WriteString(fmt.Sprintf("group = %s\n", c.Group))
	}

	// Listen.
	if c.SocketPath != "" {
		b.WriteString(fmt.Sprintf("listen = %s\n", c.SocketPath))
		if c.ListenMode != "" {
			b.WriteString(fmt.Sprintf("listen.mode = %s\n", c.ListenMode))
		}
	}

	// Process manager.
	b.WriteString("pm = dynamic\n")
	b.WriteString(fmt.Sprintf("pm.max_children = %d\n", c.MaxChildren))
	b.WriteString(fmt.Sprintf("pm.start_servers = %d\n", c.StartServers))
	b.WriteString(fmt.Sprintf("pm.min_spare_servers = %d\n", c.MinSpare))
	b.WriteString(fmt.Sprintf("pm.max_spare_servers = %d\n", c.MaxSpare))
	b.WriteString(fmt.Sprintf("pm.max_requests = %d\n", c.MaxRequests))
	b.WriteString(fmt.Sprintf("pm.process_idle_timeout = %ds\n", c.ProcessIdle))

	// Timeouts.
	b.WriteString(fmt.Sprintf("request_terminate_timeout = %ds\n", c.RequestTimeout))

	// Status.
	if c.StatusPath != "" {
		b.WriteString(fmt.Sprintf("pm.status_path = %s\n", c.StatusPath))
	}

	// Security.
	if c.ClearEnv {
		b.WriteString("clear_env = yes\n")
	} else {
		b.WriteString("clear_env = no\n")
	}
	if c.SecurityExt != "" {
		b.WriteString(fmt.Sprintf("security.limit_extensions = %s\n", c.SecurityExt))
	}

	// PHP ini.
	if c.PhpIniPath != "" {
		b.WriteString(fmt.Sprintf("php_admin_value[error_log] = %s\n", c.PhpIniPath))
	}

	// Logs.
	if c.AccessLog != "" {
		b.WriteString(fmt.Sprintf("access.log = %s\n", c.AccessLog))
		b.WriteString("access.format = \"%R - %u %t \\\"%m %r%Q%q\\\" %s %f %{mili}d %{kilo}M %C%%\"\n")
	}
	if c.ErrorLog != "" {
		b.WriteString(fmt.Sprintf("php_admin_value[error_log] = %s\n", c.ErrorLog))
	}

	// Custom directives.
	for k, v := range c.CustomDirectives {
		b.WriteString(fmt.Sprintf("php_admin_value[%s] = %s\n", k, v))
	}

	return b.String()
}

// WritePool writes the pool config to a file.
func (c *PoolConfig) WritePool(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create pool dir: %w", err)
	}

	path := filepath.Join(dir, fmt.Sprintf("%s.conf", c.Name))
	content := c.Generate()

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write pool config: %w", err)
	}

	return path, nil
}

// GenerateMainConfig produces the main php-fpm.conf.
func GenerateMainConfig(poolsDir string, pidFile string, errorLog string) string {
	var b strings.Builder

	b.WriteString("[global]\n")
	b.WriteString(fmt.Sprintf("pid = %s\n", pidFile))
	if errorLog != "" {
		b.WriteString(fmt.Sprintf("error_log = %s\n", errorLog))
	}
	b.WriteString("log_level = warning\n")
	b.WriteString("include = pools/*.conf\n")

	return b.String()
}

// WriteMainConfig writes the main php-fpm.conf.
func WriteMainConfig(dir, pidFile, errorLog string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}

	content := GenerateMainConfig(dir, pidFile, errorLog)
	path := filepath.Join(dir, "php-fpm.conf")

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write main config: %w", err)
	}

	return path, nil
}
