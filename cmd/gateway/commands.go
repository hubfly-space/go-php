package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-php/gateway/internal/config"
	"github.com/go-php/gateway/internal/deploy"
	"github.com/go-php/gateway/internal/diagnostics"
	"github.com/go-php/gateway/internal/filesystem"
	"github.com/go-php/gateway/internal/policy"
	gatewayRuntime "github.com/go-php/gateway/internal/runtime"
)

// runDoctor runs system health and readiness checks.
func runDoctor() error {
	doc := diagnostics.NewDoctor()
	report := doc.Run()

	fmt.Println("\033[1;36mGo-PHP Gateway System Doctor\033[0m")
	fmt.Printf("OS: %s | Arch: %s | Go: %s | Host: %s\n\n",
		report.OS, report.Arch, report.GoVer, report.Hostname)

	hasFail := false
	for _, check := range report.Checks {
		switch check.Status {
		case "ok":
			fmt.Printf("  \033[32m[OK]\033[0m   %-20s %s\n", check.Name, check.Message)
		case "warn":
			fmt.Printf("  \033[33m[WARN]\033[0m %-20s %s\n", check.Name, check.Message)
		case "fail":
			fmt.Printf("  \033[31m[FAIL]\033[0m %-20s %s\n", check.Name, check.Message)
			hasFail = true
		}
	}

	fmt.Println()
	if hasFail {
		return fmt.Errorf("doctor check failed")
	}
	fmt.Println("\033[32mAll critical doctor checks passed!\033[0m")
	return nil
}

// runCompat scans a project for framework and configuration compatibility.
func runCompat(args []string) error {
	targetDir := "."
	if len(args) > 0 {
		targetDir = args[0]
	}

	absDir, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("resolve dir: %w", err)
	}

	compat := diagnostics.NewCompatDoctor(absDir)
	report := compat.Scan()

	fmt.Printf("\033[1;36mGo-PHP Gateway Compatibility Scanner\033[0m\n")
	fmt.Printf("Path: %s\n", report.Root)
	if report.Framework != "" {
		fmt.Printf("Detected Framework: \033[1;32m%s\033[0m\n", report.Framework)
	}
	fmt.Printf("Compatibility Score: \033[1;33m%d/100\033[0m\n\n", report.Score)

	if len(report.Issues) > 0 {
		fmt.Println("\033[1;31mCritical Issues:\033[0m")
		for _, issue := range report.Issues {
			fmt.Printf("  - [%s] %s\n", issue.Category, issue.Message)
			if issue.Suggestion != "" {
				fmt.Printf("    \033[36mSuggestion:\033[0m %s\n", issue.Suggestion)
			}
		}
		fmt.Println()
	}

	if len(report.Warnings) > 0 {
		fmt.Println("\033[1;33mWarnings:\033[0m")
		for _, warn := range report.Warnings {
			fmt.Printf("  - [%s] %s\n", warn.Category, warn.Message)
			if warn.Suggestion != "" {
				fmt.Printf("    \033[36mSuggestion:\033[0m %s\n", warn.Suggestion)
			}
		}
		fmt.Println()
	}

	if len(report.Info) > 0 {
		fmt.Println("\033[1;34mInfo:\033[0m")
		for _, info := range report.Info {
			fmt.Printf("  - %s\n", info)
		}
		fmt.Println()
	}

	return nil
}

// runExplain traces a request through the decision pipeline.
//
// The trace is only as truthful as the components it is given. Passing a nil
// router and nil policy engine — as this command used to — leaves the route and
// policy steps permanently empty, which makes §33.1's whole purpose ("replacing
// confusing rewrite behavior") unserved. So this builds the same router,
// resolver, and policy engine that `serve` would build from the same config.
func runExplain(args []string) error {
	fs := flag.NewFlagSet("explain", flag.ExitOnError)
	configPath := fs.String("config", "", "path to gateway.yaml (default: ./gateway.yaml if present)")
	method := fs.String("method", "GET", "HTTP method to trace")
	host := fs.String("host", "", "Host header to trace (default: localhost)")
	docRootFlag := fs.String("root", ".", "document root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("usage: gateway explain [flags] <url-or-path>")
	}

	target := rest[0]
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		hostname := *host
		if hostname == "" {
			hostname = "localhost"
		}
		target = "http://" + hostname + target
	}

	req, err := http.NewRequest(*method, target, nil)
	if err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	if *host != "" {
		req.Host = *host
	}

	// Load the same config serve would. Falling back to defaults keeps the
	// command usable in a bare directory.
	cfg := config.DefaultConfig()
	path := *configPath
	if path == "" {
		if _, statErr := os.Stat("gateway.yaml"); statErr == nil {
			path = "gateway.yaml"
		}
	}
	if path != "" {
		cfg, err = config.Load(path)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		fmt.Fprintf(os.Stderr, "using config: %s\n", path)
	}

	absRoot, err := filepath.Abs(*docRootFlag)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	// Mirror serve's document-root pivot, or the file-resolution step would
	// report misses for every framework request.
	if _, pubRoot := detectFramework(absRoot); pubRoot != "" {
		absRoot = pubRoot
	}

	symlinkMode := filesystem.SymlinkWithinRoot
	if cfg.Security.SymlinkMode == "deny" {
		symlinkMode = filesystem.SymlinkDeny
	}
	protected := cfg.Security.ProtectedPatterns
	if len(protected) == 0 {
		protected = filesystem.DefaultProtectedPatterns()
	}
	resolver := filesystem.NewResolver(absRoot, symlinkMode, protected)

	routingEngine, err := buildRouter(cfg)
	if err != nil {
		return err
	}

	securityMode, err := policy.ParseMode(cfg.Security.Mode)
	if err != nil {
		return fmt.Errorf("security.mode: %w", err)
	}

	explainer := diagnostics.NewRequestExplainer(
		resolver, routingEngine, policy.NewEngineForMode(securityMode), absRoot)
	exp := explainer.Explain(req)

	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal explanation: %w", err)
	}

	fmt.Println("\033[1;36mRequest Decision Trace:\033[0m")
	fmt.Println(string(data))
	return nil
}

// runConfig handles config subcommands (validate, init).
func runConfig(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: gateway config <subcommand>")
		fmt.Println("Subcommands: validate, init")
		return nil
	}

	switch args[0] {
	case "validate":
		fs := flag.NewFlagSet("config validate", flag.ExitOnError)
		cfgPath := fs.String("config", "gateway.yaml", "path to configuration file")
		fs.Parse(args[1:])

		cfg, err := config.Load(*cfgPath)
		if err != nil {
			return fmt.Errorf("failed to load config %s: %w", *cfgPath, err)
		}

		if err := config.Validate(cfg); err != nil {
			return fmt.Errorf("config validation failed: %w", err)
		}

		fmt.Printf("\033[32m✓ Configuration %s is valid!\033[0m\n", *cfgPath)
		return nil

	case "init":
		fs := flag.NewFlagSet("config init", flag.ExitOnError)
		framework := fs.String("framework", "", "target framework (laravel, symfony, wordpress, plain)")
		out := fs.String("out", "gateway.yaml", "output file path")
		fs.Parse(args[1:])

		return runConfigInit(*framework, *out)

	default:
		return fmt.Errorf("unknown config subcommand: %s", args[0])
	}
}

// runDeploy handles deployment management commands.
func runDeploy(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: gateway deploy <subcommand>")
		fmt.Println("Subcommands: create, activate, rollback, list")
		return nil
	}

	releasesDir := "./releases"
	rm := deploy.NewReleaseManager(releasesDir)
	_ = rm.Init()

	switch args[0] {
	case "list":
		releases := rm.List()
		fmt.Println("\033[1;36mDeploy Releases:\033[0m")
		if len(releases) == 0 {
			fmt.Println("  No releases found in ./releases")
			return nil
		}

		fmt.Printf("  %-24s %-10s %-20s %s\n", "RELEASE ID", "STATE", "CREATED AT", "PHP VERSION")
		for _, r := range releases {
			fmt.Printf("  %-24s %-10s %-20s %s\n",
				r.ID, r.State, r.CreatedAt.Format("2006-01-02 15:04"), r.Version)
		}
		return nil

	case "create":
		fs := flag.NewFlagSet("deploy create", flag.ExitOnError)
		version := fs.String("version", "1.0.0", "release version")
		srcDir := fs.String("src", ".", "source directory")
		fs.Parse(args[1:])

		rel, err := rm.Create(*version, "php8.3", *srcDir, nil)
		if err != nil {
			return fmt.Errorf("create release: %w", err)
		}

		fmt.Printf("\033[32m✓ Created release %s at %s\033[0m\n", rel.ID, rel.Dir)
		return nil

	case "activate":
		if len(args) < 2 {
			return fmt.Errorf("usage: gateway deploy activate <release-id>")
		}
		relID := args[1]
		if err := rm.Activate(relID); err != nil {
			return fmt.Errorf("activate release %s: %w", relID, err)
		}
		fmt.Printf("\033[32m✓ Activated release %s\033[0m\n", relID)
		return nil

	case "rollback":
		rel, err := rm.Rollback()
		if err != nil {
			return fmt.Errorf("rollback release: %w", err)
		}
		fmt.Printf("\033[32m✓ Rolled back to previous active release %s\033[0m\n", rel.ID)
		return nil

	default:
		return fmt.Errorf("unknown deploy subcommand: %s", args[0])
	}
}

// runPHP handles PHP runtime version management.
func runPHP(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: gateway php <subcommand>")
		fmt.Println("Subcommands: list, install, use, remove")
		return nil
	}

	homeDir, _ := os.UserHomeDir()
	regRoot := filepath.Join(homeDir, ".gateway", "runtimes")
	reg := gatewayRuntime.NewRegistry(regRoot)
	_ = reg.Init()

	switch args[0] {
	case "list":
		runtimes, err := reg.List()
		if err != nil {
			return fmt.Errorf("list runtimes: %w", err)
		}

		fmt.Println("\033[1;36mPHP Runtimes (~/.gateway/runtimes):\033[0m")
		if len(runtimes) == 0 {
			fmt.Println("  No runtimes currently installed in registry.")
			fmt.Println("  System PHP-FPM binaries detected automatically by `gateway serve`.")
			return nil
		}

		for _, rt := range runtimes {
			fmt.Printf("  - %s (version: %s, platform: %s)\n", rt.ID, rt.Version, rt.Platform)
		}
		return nil

	case "use":
		if len(args) < 2 {
			return fmt.Errorf("usage: gateway php use <runtime-id-or-version>")
		}
		target := args[1]
		if err := reg.Use(gatewayRuntime.RuntimeID(target)); err != nil {
			return fmt.Errorf("use runtime %s: %w", target, err)
		}
		fmt.Printf("\033[32m✓ Switched active runtime to %s\033[0m\n", target)
		return nil

	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: gateway php remove <runtime-id>")
		}
		target := args[1]
		if err := reg.Remove(gatewayRuntime.RuntimeID(target)); err != nil {
			return fmt.Errorf("remove runtime %s: %w", target, err)
		}
		fmt.Printf("\033[32m✓ Removed runtime %s\033[0m\n", target)
		return nil

	case "install":
		if len(args) < 2 {
			return fmt.Errorf("usage: gateway php install <version>")
		}
		ver := args[1]
		fmt.Printf("Installing PHP %s...\n", ver)
		manifest := &gatewayRuntime.Manifest{
			Version:  ver,
			Platform: runtime.GOOS,
			Arch:     runtime.GOARCH,
			Flavor:   "standard",
		}
		// Register stub manifest if local directory created
		tmpDir, err := os.MkdirTemp("", "gateway-php-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)

		rt, err := reg.Install(manifest, tmpDir, nil)
		if err != nil {
			return fmt.Errorf("install runtime %s: %w", ver, err)
		}
		fmt.Printf("\033[32m✓ Installed PHP %s (ID: %s)\033[0m\n", ver, rt.ID)
		return nil

	default:
		return fmt.Errorf("unknown php subcommand: %s", args[0])
	}
}

// runIncident handles incident snapshot creation.
func runIncident(args []string) error {
	reason := "manual incident capture"
	if len(args) > 0 && args[0] == "capture" {
		if len(args) > 1 {
			reason = strings.Join(args[1:], " ")
		}
	}

	snap := diagnostics.Capture(reason)
	filename := fmt.Sprintf("incident_%s.json", snap.ID)
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	if err := os.WriteFile(filename, data, 0600); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}

	fmt.Printf("\033[32m✓ Captured incident snapshot to %s\033[0m (ID: %s)\n", filename, snap.ID)
	return nil
}

// runService handles systemd service file installation.
func runService(args []string) error {
	serviceContent := `[Unit]
Description=Go-PHP Gateway Server
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/gateway serve --config /etc/gateway/gateway.yaml
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`
	target := "/etc/systemd/system/gateway.service"
	if err := os.WriteFile(target, []byte(serviceContent), 0644); err != nil {
		fmt.Println("\033[1;33mSystemd service template:\033[0m")
		fmt.Println(serviceContent)
		fmt.Printf("\033[33mNote: Could not write directly to %s (permission denied). Run with sudo or save manually.\033[0m\n", target)
		return nil
	}

	fmt.Printf("\033[32m✓ Systemd service installed to %s\033[0m\n", target)
	fmt.Println("To enable and start:")
	fmt.Println("  sudo systemctl daemon-reload")
	fmt.Println("  sudo systemctl enable --now gateway")
	return nil
}
