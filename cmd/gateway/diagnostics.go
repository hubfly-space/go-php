package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-php/gateway/internal/config"
	"github.com/go-php/gateway/internal/diagnostics"

	"gopkg.in/yaml.v3"
)

// These three commands are thin CLI surfaces over libraries that already
// existed, were well tested, and were reachable from nothing:
// diagnostics.HtaccessTranslator (§13.4, §33.2), diagnostics.ContractTestSuite
// (§33.7), and diagnostics.ShadowTester (§33.3).

// runMigrate translates Apache configuration into gateway routes.
func runMigrate(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: gateway migrate <subcommand>")
		fmt.Println("Subcommands: htaccess")
		return nil
	}

	switch args[0] {
	case "htaccess":
		return runMigrateHtaccess(args[1:])
	default:
		return fmt.Errorf("unknown migrate subcommand: %s", args[0])
	}
}

func runMigrateHtaccess(args []string) error {
	fs := flag.NewFlagSet("migrate htaccess", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit JSON instead of YAML")
	if err := fs.Parse(args); err != nil {
		return err
	}

	target := "."
	if rest := fs.Args(); len(rest) > 0 {
		target = rest[0]
	}

	// Accept either a directory containing .htaccess or the file itself.
	path := target
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		path = filepath.Join(target, ".htaccess")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	translator := diagnostics.NewHtaccessTranslator()
	routes, warnings := translator.Translate(string(content))

	// §13.4: "Do not silently ignore security-related directives." Warnings go
	// to stderr so the translated routes on stdout stay pipeable.
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "\033[33mwarning:\033[0m %s\n", warning)
	}

	if len(routes) == 0 {
		fmt.Fprintf(os.Stderr, "no translatable directives found in %s\n", path)
		return nil
	}

	if *asJSON {
		out, err := json.MarshalIndent(routes, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal routes: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	// Emit a routes block that can be pasted into gateway.yaml.
	out, err := yaml.Marshal(map[string]any{"routes": routes})
	if err != nil {
		return fmt.Errorf("marshal routes: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\n# translated from %s — review before use\n", path)
	fmt.Print(string(out))
	return nil
}

// runTest runs declarative checks against the configured routes.
func runTest(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: gateway test <subcommand>")
		fmt.Println("Subcommands: routes")
		return nil
	}

	switch args[0] {
	case "routes":
		return runTestRoutes(args[1:])
	default:
		return fmt.Errorf("unknown test subcommand: %s", args[0])
	}
}

// contractFile is the on-disk shape of a contract test file.
type contractFile struct {
	Tests []diagnostics.ContractTest `yaml:"contract_tests" json:"contract_tests"`
}

func runTestRoutes(args []string) error {
	fs := flag.NewFlagSet("test routes", flag.ExitOnError)
	configPath := fs.String("config", "gateway.yaml", "path to gateway.yaml")
	testsPath := fs.String("tests", "", "path to a contract test file (default: built-in standard tests)")
	asJSON := fs.Bool("json", false, "emit JSON results")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	engine, err := buildRouter(cfg)
	if err != nil {
		return err
	}

	suite := diagnostics.NewContractTestSuite(engine)

	if *testsPath == "" {
		for _, test := range diagnostics.GenerateStandardTests() {
			suite.AddTest(test)
		}
	} else {
		data, readErr := os.ReadFile(*testsPath)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", *testsPath, readErr)
		}
		var file contractFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			return fmt.Errorf("parse %s: %w", *testsPath, err)
		}
		if len(file.Tests) == 0 {
			return fmt.Errorf("%s defines no contract_tests", *testsPath)
		}
		for _, test := range file.Tests {
			suite.AddTest(test)
		}
	}

	results := suite.RunAll()

	if *asJSON {
		out, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal results: %w", err)
		}
		fmt.Println(string(out))
	} else {
		for _, r := range results {
			if r.Passed {
				fmt.Printf("  \033[32mPASS\033[0m  %s\n", r.Name)
			} else {
				fmt.Printf("  \033[31mFAIL\033[0m  %s — %s\n", r.Name, r.Error)
			}
		}
		fmt.Println()
		fmt.Println(suite.Summary(results))
	}

	// A failing contract must fail the command, or this is unusable in CI.
	for _, r := range results {
		if !r.Passed {
			return fmt.Errorf("%d of %d route contracts failed", countFailed(results), len(results))
		}
	}
	return nil
}

func countFailed(results []diagnostics.ContractTestResult) int {
	n := 0
	for _, r := range results {
		if !r.Passed {
			n++
		}
	}
	return n
}

// runShadow compares an active runtime against a candidate (§33.3).
func runShadow(args []string) error {
	fs := flag.NewFlagSet("shadow", flag.ExitOnError)
	activeURL := fs.String("active", "", "base URL of the active runtime (required)")
	candidateURL := fs.String("candidate", "", "base URL of the candidate runtime (required)")
	timeout := fs.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := fs.Bool("json", false, "emit JSON results")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *activeURL == "" || *candidateURL == "" {
		return fmt.Errorf("usage: gateway shadow --active <url> --candidate <url> [paths...]")
	}

	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"/"}
	}

	tester := diagnostics.NewShadowTester(*activeURL, *candidateURL)

	for _, path := range paths {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		// Only idempotent requests are duplicated: §33.3 requires that
		// state-changing requests are never sent to a candidate.
		result, err := tester.Compare(ctx, "GET", path, nil)
		cancel()

		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[31merror\033[0m %s: %v\n", path, err)
			continue
		}
		if !*asJSON {
			mark := "\033[32mmatch\033[0m"
			if !result.StatusMatch || !result.BodyMatch {
				mark = "\033[31mDIFFER\033[0m"
			}
			fmt.Printf("  %s  %s (active %d, candidate %d)\n",
				mark, path, result.ActiveStatus, result.CandidateStatus)
		}
	}

	summary := tester.Summary()

	if *asJSON {
		out, err := json.MarshalIndent(map[string]any{
			"results": tester.Results(),
			"summary": summary,
		}, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal results: %w", err)
		}
		fmt.Println(string(out))
	} else {
		fmt.Println()
		fmt.Println(summary.String())
	}

	if !summary.IsSafe() {
		return fmt.Errorf("candidate runtime differs from active; not safe to promote")
	}
	return nil
}
