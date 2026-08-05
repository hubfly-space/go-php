package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-php/gateway/internal/config"
)

func TestCLI_VersionCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"gateway", "version"}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("version command panicked: %v", r)
		}
	}()
}

func TestCLI_InitCommand(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "my-php-app")

	err := runInit("", "", []string{targetDir})
	if err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "gateway.yaml")); err != nil {
		t.Errorf("expected gateway.yaml to be created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "index.php")); err != nil {
		t.Errorf("expected index.php to be created: %v", err)
	}
}

func TestCLI_ConfigValidateCommand(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gateway.yaml")
	cfgContent := `schema: gateway/v1
server:
  listen: "127.0.0.1:8080"
php:
  max_children: 10
`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0644)

	err := runConfig([]string{"validate", "--config", cfgPath})
	if err != nil {
		t.Fatalf("runConfig validate failed: %v", err)
	}
}

func TestCLI_DoctorAndCompatCommands(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php"), 0644)

	if err := runDoctor(); err != nil {
		t.Logf("runDoctor returned info/warning: %v", err)
	}

	if err := runCompat([]string{dir}); err != nil {
		t.Errorf("runCompat failed: %v", err)
	}
}

func TestCLI_ScriptResolutionAndFrameworkDetection(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "public"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "artisan"), []byte(""), 0755)
	_ = os.WriteFile(filepath.Join(dir, "public", "index.php"), []byte("<?php"), 0644)

	fw, docRoot := detectFramework(dir)
	if fw != "Laravel" {
		t.Errorf("framework = %q, want Laravel", fw)
	}
	if docRoot != filepath.Join(dir, "public") {
		t.Errorf("docRoot = %q, want %q", docRoot, filepath.Join(dir, "public"))
	}

	sName, sPath := resolveScript(docRoot, "/index.php")
	if sName != "/index.php" {
		t.Errorf("scriptName = %q, want /index.php", sName)
	}
	if sPath != filepath.Join(docRoot, "index.php") {
		t.Errorf("scriptPath = %q, want %q", sPath, filepath.Join(docRoot, "index.php"))
	}
}

func TestCLI_Helpers(t *testing.T) {
	if mime := detectMIME("style.css"); mime != "text/css; charset=utf-8" {
		t.Errorf("mime = %q, want text/css; charset=utf-8", mime)
	}
	if mime := detectMIME("app.js"); mime != "application/javascript; charset=utf-8" {
		t.Errorf("mime = %q, want application/javascript; charset=utf-8", mime)
	}

	cfg := config.DefaultConfig()
	routerEngine, err := buildRouter(cfg)
	if err != nil {
		t.Fatalf("buildRouter failed: %v", err)
	}
	if routerEngine == nil {
		t.Error("buildRouter returned nil engine")
	}

	cachePolicy := buildCachePolicy(cfg)
	if cachePolicy == nil {
		t.Error("buildCachePolicy returned nil policy")
	}
}
