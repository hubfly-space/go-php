package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CompatibilityReport holds the results of a compatibility scan.
type CompatibilityReport struct {
	Root       string        `json:"root"`
	ScannedAt  string        `json:"scanned_at"`
	Framework  string        `json:"framework,omitempty"`
	PHPVersion string        `json:"php_version,omitempty"`
	Issues     []CompatIssue `json:"issues"`
	Warnings   []CompatIssue `json:"warnings"`
	Info       []string      `json:"info"`
	Score      int           `json:"score"` // 0-100
}

// CompatIssue is a single compatibility finding.
type CompatIssue struct {
	Category   string `json:"category"`
	Severity   string `json:"severity"` // error, warning, info
	File       string `json:"file,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// CompatDoctor scans a project for compatibility issues.
type CompatDoctor struct {
	root string
}

// NewCompatDoctor creates a compatibility doctor.
func NewCompatDoctor(root string) *CompatDoctor {
	return &CompatDoctor{root: root}
}

// Scan performs a full compatibility scan.
func (d *CompatDoctor) Scan() *CompatibilityReport {
	report := &CompatibilityReport{
		Root:      d.root,
		ScannedAt: "now",
	}

	d.detectFramework(report)
	d.checkHTAccess(report)
	d.checkPublicDir(report)
	d.checkPHPFiles(report)
	d.checkConfigFiles(report)
	d.checkWritableDirs(report)
	d.checkRiskyFiles(report)
	d.checkExtensions(report)

	// Calculate score.
	report.Score = d.calculateScore(report)
	return report
}

func (d *CompatDoctor) detectFramework(r *CompatibilityReport) {
	checks := []struct {
		file string
		name string
	}{
		{"artisan", "Laravel"},
		{"public/index.php", "Laravel"},
		{"bin/console", "Symfony"},
		{"wp-config.php", "WordPress"},
		{"wp-login.php", "WordPress"},
		{"composer.json", "PHP/Composer"},
		{"package.json", "Node.js"},
		{"Gemfile", "Ruby"},
		{"requirements.txt", "Python"},
	}

	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(d.root, c.file)); err == nil {
			r.Framework = c.name
			r.Info = append(r.Info, fmt.Sprintf("Framework detected: %s", c.name))
			return
		}
	}
}

func (d *CompatDoctor) checkHTAccess(r *CompatibilityReport) {
	htaccessFiles := []string{".htaccess", "public/.htaccess"}

	for _, f := range htaccessFiles {
		path := filepath.Join(d.root, f)
		if _, err := os.Stat(path); err == nil {
			data, _ := os.ReadFile(path)
			content := string(data)

			r.Warnings = append(r.Warnings, CompatIssue{
				Category:   "htaccess",
				Severity:   "warning",
				File:       f,
				Message:    ".htaccess file found — gateway does not process .htaccess",
				Suggestion: "Convert rewrite rules to gateway.yaml routes",
			})

			// Check for common directives.
			htaccessDirectives := []string{"RewriteRule", "RewriteCond", "Redirect", "AuthType"}
			for _, directive := range htaccessDirectives {
				if strings.Contains(content, directive) {
					r.Warnings = append(r.Warnings, CompatIssue{
						Category:   "htaccess",
						Severity:   "warning",
						File:       f,
						Message:    fmt.Sprintf("Uses %s directive", directive),
						Suggestion: fmt.Sprintf("Convert %s to equivalent gateway.yaml route", directive),
					})
				}
			}
		}
	}
}

func (d *CompatDoctor) checkPublicDir(r *CompatibilityReport) {
	publicDir := filepath.Join(d.root, "public")
	info, err := os.Stat(publicDir)
	if err != nil {
		r.Warnings = append(r.Warnings, CompatIssue{
			Category:   "structure",
			Severity:   "warning",
			Message:    "No public/ directory found",
			Suggestion: "Create a public/ directory for web-accessible files",
		})
		return
	}

	if !info.IsDir() {
		r.Issues = append(r.Issues, CompatIssue{
			Category: "structure",
			Severity: "error",
			Message:  "public exists but is not a directory",
		})
		return
	}

	r.Info = append(r.Info, "public/ directory found")

	// Check for index.php in public.
	if _, err := os.Stat(filepath.Join(publicDir, "index.php")); err == nil {
		r.Info = append(r.Info, "public/index.php found")
	} else {
		r.Warnings = append(r.Warnings, CompatIssue{
			Category:   "structure",
			Severity:   "warning",
			Message:    "No index.php in public/",
			Suggestion: "Add public/index.php as the front controller",
		})
	}
}

func (d *CompatDoctor) checkPHPFiles(r *CompatibilityReport) {
	var phpFiles []string

	filepath.Walk(d.root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".php") {
			rel, _ := filepath.Rel(d.root, path)
			phpFiles = append(phpFiles, rel)
		}
		return nil
	})

	r.Info = append(r.Info, fmt.Sprintf("Found %d PHP files", len(phpFiles)))

	// Check for common issues in PHP files.
	for _, f := range phpFiles {
		path := filepath.Join(d.root, f)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)

		// Check for deprecated functions.
		deprecated := []string{"each(", "create_function(", "ereg(", "eregi("}
		for _, fn := range deprecated {
			if strings.Contains(content, fn) {
				r.Warnings = append(r.Warnings, CompatIssue{
					Category: "php",
					Severity: "warning",
					File:     f,
					Message:  fmt.Sprintf("Uses deprecated function %s", fn),
				})
			}
		}

		// Check for short open tags.
		if strings.Contains(content, "<?=") || strings.Contains(content, "<? ") {
			r.Warnings = append(r.Warnings, CompatIssue{
				Category: "php",
				Severity: "warning",
				File:     path,
				Message:  "Uses PHP short open tags (<?= or <?)",
			})
		}
	}
}

func (d *CompatDoctor) checkConfigFiles(r *CompatibilityReport) {
	configFiles := []struct {
		name     string
		severity string
		message  string
	}{
		{".env", "info", ".env file found — ensure it's not publicly accessible"},
		{"php.ini", "info", "php.ini found in project root"},
		{".user.ini", "warning", ".user.ini found — gateway does not process .user.ini"},
		{"auth.json", "warning", "auth.json found — should not be publicly accessible"},
		{"composer.json", "info", "composer.json found"},
		{"composer.lock", "info", "composer.lock found"},
	}

	for _, cf := range configFiles {
		if _, err := os.Stat(filepath.Join(d.root, cf.name)); err == nil {
			r.Info = append(r.Info, fmt.Sprintf("%s found", cf.name))
			if cf.severity == "warning" {
				r.Warnings = append(r.Warnings, CompatIssue{
					Category: "config",
					Severity: cf.severity,
					File:     cf.name,
					Message:  cf.message,
				})
			}
		}
	}
}

func (d *CompatDoctor) checkWritableDirs(r *CompatibilityReport) {
	writableDirs := []string{"storage", "cache", "tmp", "logs", "vendor"}

	for _, dir := range writableDirs {
		path := filepath.Join(d.root, dir)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			r.Info = append(r.Info, fmt.Sprintf("Writable directory: %s/", dir))
		}
	}
}

func (d *CompatDoctor) checkRiskyFiles(r *CompatibilityReport) {
	risky := []struct {
		pattern string
		message string
	}{
		{".git", "Git directory found in web root"},
		{".svn", "SVN directory found in web root"},
		{".env", ".env file may contain secrets"},
		{"*.sql", "SQL dump file found"},
		{"*.log", "Log file found"},
		{"*.bak", "Backup file found"},
	}

	for _, rk := range risky {
		matches, _ := filepath.Glob(filepath.Join(d.root, "**", rk.pattern))
		if len(matches) > 0 {
			r.Warnings = append(r.Warnings, CompatIssue{
				Category:   "security",
				Severity:   "warning",
				Message:    rk.message,
				Suggestion: "Ensure these files are not publicly accessible",
			})
		}
	}
}

func (d *CompatDoctor) checkExtensions(r *CompatibilityReport) {
	// Check composer.json for required extensions.
	composerPath := filepath.Join(d.root, "composer.json")
	if data, err := os.ReadFile(composerPath); err == nil {
		content := string(data)
		if strings.Contains(content, "ext-") {
			r.Info = append(r.Info, "composer.json requires PHP extensions (check with `composer check-platform-reqs`)")
		}
	}
}

func (d *CompatDoctor) calculateScore(r *CompatibilityReport) int {
	score := 100

	for _, issue := range r.Issues {
		switch issue.Severity {
		case "error":
			score -= 20
		case "warning":
			score -= 5
		}
	}

	for _, w := range r.Warnings {
		if w.Severity == "warning" {
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}
