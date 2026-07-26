package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompatDoctor_ScanEmpty(t *testing.T) {
	dir := t.TempDir()

	doctor := NewCompatDoctor(dir)
	report := doctor.Scan()

	if report.Score < 95 {
		t.Errorf("expected score >= 95 for empty dir, got %d", report.Score)
	}
}

func TestCompatDoctor_Laravel(t *testing.T) {
	dir := t.TempDir()

	// Create Laravel structure.
	os.MkdirAll(filepath.Join(dir, "public"), 0755)
	os.WriteFile(filepath.Join(dir, "artisan"), []byte(`#!/usr/bin/env php`), 0755)
	os.WriteFile(filepath.Join(dir, "public", "index.php"), []byte(`<?php echo "Laravel"; ?>`), 0644)
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require": {"php": ">=8.1"}}`), 0644)

	doctor := NewCompatDoctor(dir)
	report := doctor.Scan()

	if report.Framework != "Laravel" {
		t.Errorf("expected Laravel, got %s", report.Framework)
	}
	t.Logf("framework: %s, score: %d", report.Framework, report.Score)
}

func TestCompatDoctor_WordPress(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "wp-config.php"), []byte(`<?php ?>`), 0644)
	os.WriteFile(filepath.Join(dir, "wp-login.php"), []byte(`<?php ?>`), 0644)

	doctor := NewCompatDoctor(dir)
	report := doctor.Scan()

	if report.Framework != "WordPress" {
		t.Errorf("expected WordPress, got %s", report.Framework)
	}
}

func TestCompatDoctor_HtAccess(t *testing.T) {
	dir := t.TempDir()

	// Create .htaccess with rewrite rules.
	htaccess := `RewriteEngine On
RewriteRule ^(.*)$ index.php [L]`
	os.WriteFile(filepath.Join(dir, ".htaccess"), []byte(htaccess), 0644)

	doctor := NewCompatDoctor(dir)
	report := doctor.Scan()

	// Should have at least one warning about .htaccess.
	foundHtaccess := false
	for _, w := range report.Warnings {
		if w.Category == "htaccess" {
			foundHtaccess = true
			break
		}
	}
	if !foundHtaccess {
		t.Error("expected .htaccess warning")
	}
}

func TestCompatDoctor_SecurityIssues(t *testing.T) {
	dir := t.TempDir()

	// Create risky files.
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(`[core]`), 0644)
	os.WriteFile(filepath.Join(dir, ".env"), []byte(`SECRET=abc`), 0644)
	os.WriteFile(filepath.Join(dir, "dump.sql"), []byte(`SELECT * FROM users`), 0644)
	os.WriteFile(filepath.Join(dir, "backup.log"), []byte(`log entry`), 0644)

	doctor := NewCompatDoctor(dir)
	report := doctor.Scan()

	if report.Score >= 100 {
		t.Errorf("expected score < 100 for risky files, got %d", report.Score)
	}
	t.Logf("score: %d, warnings: %d", report.Score, len(report.Warnings))
}

func TestCompatDoctor_DeprecatedPHP(t *testing.T) {
	dir := t.TempDir()

	// Create PHP file with deprecated function.
	php := `<?php each($array); create_function('$a', 'return $a;'); ?>`
	os.WriteFile(filepath.Join(dir, "old.php"), []byte(php), 0644)

	doctor := NewCompatDoctor(dir)
	report := doctor.Scan()

	foundDeprecated := false
	for _, w := range report.Warnings {
		if w.Category == "php" {
			foundDeprecated = true
			break
		}
	}
	if !foundDeprecated {
		t.Error("expected deprecated PHP function warning")
	}
}

func TestCompatDoctor_ScoreCalculation(t *testing.T) {
	dir := t.TempDir()

	// Perfect project: public dir with index.php, no risky files.
	os.MkdirAll(filepath.Join(dir, "public"), 0755)
	os.WriteFile(filepath.Join(dir, "public", "index.php"), []byte(`<?php ?>`), 0644)

	doctor := NewCompatDoctor(dir)
	report := doctor.Scan()

	if report.Score < 80 {
		t.Errorf("expected score >= 80 for clean project, got %d", report.Score)
	}
}
