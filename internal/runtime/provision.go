package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// extensionPackageMap maps PHP extension names to Debian package name suffixes.
// The full package name is php{version}-{suffix} (e.g., php8.3-mysql).
var extensionPackageMap = map[string]string{
	"mysqli":      "mysql",
	"pdo_mysql":   "mysql",
	"mysqlnd":     "mysql",
	"gd":          "gd",
	"bcmath":      "bcmath",
	"curl":        "curl",
	"mbstring":    "mbstring",
	"intl":        "intl",
	"zip":         "zip",
	"xml":         "xml",
	"xmlwriter":   "xml",
	"dom":         "xml",
	"simplexml":   "xml",
	"pdo_sqlite":  "sqlite3",
	"sqlite3":     "sqlite3",
	"pdo_pgsql":   "pgsql",
	"pgsql":       "pgsql",
	"opcache":     "opcache",
	"xdebug":      "xdebug",
	"pcov":        "pcov",
	"sodium":      "sodium",
	"openssl":     "openssl",
	"sockets":     "common",
	"tokenizer":   "common",
	"json":        "common",
	"exif":        "common",
	"fileinfo":    "common",
	"ctype":       "common",
	"filter":      "common",
	"hash":        "common",
	"pdo":         "mysql",
}

// Provisioner handles detection and installation of OS packages for PHP extensions.
type Provisioner struct {
	phpVersion      string
	extensionDir    string
	packagePrefix   string
}

// NewProvisioner creates a provisioner by inspecting the given PHP-FPM binary path.
// binary can be a full path like /usr/sbin/php-fpm8.3 or just php-fpm8.3.
func NewProvisioner(binary string) *Provisioner {
	version := detectPHPVersion(binary)
	prefix := fmt.Sprintf("php%s", version)
	extDir := detectExtensionDir(binary, version)
	return &Provisioner{
		phpVersion:    version,
		extensionDir:  extDir,
		packagePrefix: prefix,
	}
}

// detectPHPVersion extracts the PHP version from a binary path.
func detectPHPVersion(binary string) string {
	re := regexp.MustCompile(`(\d+\.\d+)`)
	matches := re.FindStringSubmatch(binary)
	if len(matches) > 1 {
		return matches[1]
	}
	// Try php-config --version.
	if out, err := exec.Command("php-config", "--version").Output(); err == nil {
		v := strings.TrimSpace(string(out))
		if matches := re.FindStringSubmatch(v); len(matches) > 1 {
			return matches[1]
		}
	}
	// Try php --version.
	if out, err := exec.Command("php", "--version").Output(); err == nil {
		lines := strings.SplitN(string(out), "\n", 2)
		if len(lines) > 0 {
			if matches := re.FindStringSubmatch(lines[0]); len(matches) > 1 {
				return matches[1]
			}
		}
	}
	return "8.3"
}

// detectExtensionDir finds the PHP extension directory for the given version.
func detectExtensionDir(binary, version string) string {
	if binary != "" {
		if out, err := exec.Command(binary, "-i").Output(); err == nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				if strings.Contains(line, "extension_dir") {
					parts := strings.SplitN(line, "=>", 2)
					if len(parts) == 2 {
						dir := strings.TrimSpace(parts[1])
						if info, err := os.Stat(dir); err == nil && info.IsDir() {
							return dir
						}
					}
				}
			}
		}
	}
	// Common paths by version.
	versionDirs := map[string]string{
		"8.3": "/usr/lib/php/20230831",
		"8.2": "/usr/lib/php/20220829",
		"8.1": "/usr/lib/php/20210902",
		"8.0": "/usr/lib/php/20190902",
	}
	if dir, ok := versionDirs[version]; ok {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	// Fallback: find the newest extension directory.
	base := "/usr/lib/php"
	if entries, err := os.ReadDir(base); err == nil {
		for i := len(entries) - 1; i >= 0; i-- {
			path := filepath.Join(base, entries[i].Name())
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				return path
			}
		}
	}
	return ""
}

// PackageName returns the Debian package name for a given extension.
func (p *Provisioner) PackageName(extensionName string) string {
	suffix, ok := extensionPackageMap[extensionName]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s-%s", p.packagePrefix, suffix)
}

// MissingExtensions returns extensions whose .so files are not found in the extension directory.
func (p *Provisioner) MissingExtensions(names []string) []string {
	if p.extensionDir == "" {
		return names
	}
	var missing []string
	for _, name := range names {
		soPath := filepath.Join(p.extensionDir, name+".so")
		if _, err := os.Stat(soPath); os.IsNotExist(err) {
			missing = append(missing, name)
		}
	}
	return missing
}

// Provision installs the OS packages for extensions whose .so files are missing.
// Uses apt-get install via sudo.
func (p *Provisioner) Provision(ctx context.Context, extensions []string) (installed []string, errors []error) {
	missing := p.MissingExtensions(extensions)
	if len(missing) == 0 {
		return nil, nil
	}

	// Collect unique package names.
	pkgSet := make(map[string]string) // package name -> extensions
	for _, ext := range missing {
		pkg := p.PackageName(ext)
		if pkg == "" {
			errors = append(errors, fmt.Errorf("no known package for extension %q", ext))
			continue
		}
		pkgSet[pkg] = ext
		installed = append(installed, ext)
	}

	if len(pkgSet) == 0 {
		return installed, errors
	}

	var pkgs []string
	for pkg := range pkgSet {
		pkgs = append(pkgs, pkg)
	}

	if err := p.installPackages(ctx, pkgs); err != nil {
		errors = append(errors, fmt.Errorf("install packages %v: %w", pkgs, err))
		return installed, errors
	}

	return installed, nil
}

// installPackages runs apt-get install for the given packages via sudo.
func (p *Provisioner) installPackages(ctx context.Context, pkgs []string) error {
	args := append([]string{"apt-get", "install", "-y", "-qq"}, pkgs...)
	cmd := exec.CommandContext(ctx, "sudo", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
