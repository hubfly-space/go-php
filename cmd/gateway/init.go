package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runInit initializes a new Go-PHP Gateway project environment.
func runInit(frameworkFlag, phpVerFlag string, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve dir: %w", err)
	}

	if err := os.MkdirAll(absDir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	fmt.Printf("\033[1;36mInitializing Go-PHP Gateway project at:\033[0m %s\n", absDir)

	// Framework detection if not explicitly specified.
	framework := frameworkFlag
	if framework == "" {
		detected, _ := detectFramework(absDir)
		if detected != "" {
			framework = detected
			fmt.Printf("  \033[32m✓ Detected framework:\033[0m %s\n", framework)
		} else {
			framework = "plain"
			fmt.Println("  \033[33m! No specific framework detected, using default PHP configuration\033[0m")
		}
	} else {
		fmt.Printf("  \033[32m✓ Using specified framework:\033[0m %s\n", framework)
	}

	// 1. Generate gateway.yaml
	configPath := filepath.Join(absDir, "gateway.yaml")
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("  \033[33m! gateway.yaml already exists, skipping creation\033[0m\n")
	} else {
		if err := writeDefaultConfig(configPath, framework); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
		fmt.Printf("  \033[32m✓ Created gateway.yaml\033[0m\n")
	}

	// 2. Generate .gateway.lock
	lockPath := filepath.Join(absDir, ".gateway.lock")
	if _, err := os.Stat(lockPath); err == nil {
		fmt.Printf("  \033[33m! .gateway.lock already exists, skipping creation\033[0m\n")
	} else {
		phpVer := phpVerFlag
		if phpVer == "" {
			phpVer = "8.3"
		}
		lockContent := fmt.Sprintf(`# Go-PHP Gateway Lock File
schema: "1.0"
project: %s
php_version: "%s"
generated_at: "%s"
`, filepath.Base(absDir), phpVer, "2026-07-28")
		if err := os.WriteFile(lockPath, []byte(lockContent), 0644); err != nil {
			return fmt.Errorf("write lockfile: %w", err)
		}
		fmt.Printf("  \033[32m✓ Created .gateway.lock (PHP %s)\033[0m\n", phpVer)
	}

	// 3. Create sample index.php if empty directory
	entries, err := os.ReadDir(absDir)
	if err == nil && len(entries) <= 2 { // only gateway.yaml and .gateway.lock
		samplePath := filepath.Join(absDir, "index.php")
		if strings.ToLower(framework) == "laravel" || strings.ToLower(framework) == "symfony" {
			pubDir := filepath.Join(absDir, "public")
			os.MkdirAll(pubDir, 0755)
			samplePath = filepath.Join(pubDir, "index.php")
		}

		sampleContent := `<?php
echo "<h1>Hello from Go-PHP Gateway!</h1>";
echo "<p>PHP Version: " . phpversion() . "</p>";
echo "<p>Server Software: " . ($_SERVER['SERVER_SOFTWARE'] ?? 'Go-PHP Gateway') . "</p>";
`
		if _, err := os.Stat(samplePath); err != nil {
			_ = os.WriteFile(samplePath, []byte(sampleContent), 0644)
			fmt.Printf("  \033[32m✓ Created sample %s\033[0m\n", filepath.Base(samplePath))
		}
	}

	fmt.Println("\n\033[1;32mProject initialized successfully!\033[0m")
	fmt.Println("To start the server, run:")
	fmt.Printf("  \033[36mgateway serve %s\033[0m\n\n", dir)
	return nil
}

// runConfigInit generates a starter gateway.yaml file.
func runConfigInit(frameworkFlag, outputPath string) error {
	if outputPath == "" {
		outputPath = "gateway.yaml"
	}

	absPath, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	if _, err := os.Stat(absPath); err == nil {
		return fmt.Errorf("file %s already exists", outputPath)
	}

	framework := frameworkFlag
	if framework == "" {
		dir := filepath.Dir(absPath)
		detected, _ := detectFramework(dir)
		if detected != "" {
			framework = detected
		} else {
			framework = "plain"
		}
	}

	if err := writeDefaultConfig(absPath, framework); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("\033[32m✓ Starter config written to %s (framework: %s)\033[0m\n", outputPath, framework)
	return nil
}

func writeDefaultConfig(path, framework string) error {
	fw := strings.ToLower(framework)
	var routesYAML string

	switch fw {
	case "laravel", "symfony":
		routesYAML = `routes:
  - path_prefix: "/assets"
    target: "public/assets"
  - path_prefix: "/"
    target: "public/index.php"
`
	case "wordpress":
		routesYAML = `routes:
  - path_prefix: "/wp-admin"
    target: "wp-admin/index.php"
  - path_prefix: "/"
    target: "index.php"
`
	default:
		routesYAML = `routes:
  - path_prefix: "/"
    target: "index.php"
`
	}

	content := fmt.Sprintf(`# Go-PHP Gateway Configuration
schema: "1.0"

server:
  addr: "127.0.0.1:8080"
  read_timeout: "30s"
  write_timeout: "60s"
  read_header_timeout: "10s"
  idle_timeout: "120s"
  max_header_bytes: 1048576

php:
  binary: "/usr/sbin/php-fpm"
  max_children: 10
  start_servers: 2
  min_spare: 1
  max_spare: 3
  request_timeout: "30s"

%s
logging:
  format: "json"
  level: "info"

security:
  symlink_mode: "within_root"
  max_body_size: 10485760
  protected_patterns:
    - ".env*"
    - ".git/**"
    - ".svn"
    - "*.sql"
    - "*.sqlite*"
    - "*.log"
    - "*.bak"
    - "composer.json"
    - "gateway.yaml"
`, routesYAML)

	return os.WriteFile(path, []byte(content), 0644)
}
