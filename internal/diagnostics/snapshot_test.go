package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCapture(t *testing.T) {
	snap := Capture("test incident")

	if snap.ID == "" {
		t.Error("expected non-empty ID")
	}
	if snap.Reason != "test incident" {
		t.Errorf("reason = %q", snap.Reason)
	}
	if snap.System.OS == "" {
		t.Error("expected OS to be set")
	}
	if snap.System.NumCPU == 0 {
		t.Error("expected NumCPU > 0")
	}
	if snap.System.PID == 0 {
		t.Error("expected PID > 0")
	}
}

func TestSnapshotAddError(t *testing.T) {
	snap := Capture("test")
	snap.AddError("error", "something failed", "req_123")
	snap.AddError("warn", "slow query", "")

	if len(snap.Errors) != 2 {
		t.Errorf("errors = %d, want 2", len(snap.Errors))
	}
	if snap.Errors[0].RequestID != "req_123" {
		t.Errorf("request_id = %q", snap.Errors[0].RequestID)
	}
}

func TestSnapshotSetHealth(t *testing.T) {
	snap := Capture("test")
	snap.SetHealth("php-fpm", "ok")
	snap.SetHealth("database", "error")

	if snap.Health["php-fpm"] != "ok" {
		t.Errorf("php-fpm health = %q", snap.Health["php-fpm"])
	}
}

func TestSnapshotSetConfig(t *testing.T) {
	snap := Capture("test")
	snap.SetConfig(map[string]interface{}{
		"addr":  ":8080",
		"token": "secret123",
		"nested": map[string]interface{}{
			"password": "hunter2",
			"port":     3306,
		},
	})

	if snap.Config["addr"] != ":8080" {
		t.Errorf("addr = %v", snap.Config["addr"])
	}
	if snap.Config["token"] != "***REDACTED***" {
		t.Errorf("token should be redacted, got %v", snap.Config["token"])
	}

	nested, ok := snap.Config["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("nested should be a map")
	}
	if nested["password"] != "***REDACTED***" {
		t.Errorf("nested password should be redacted, got %v", nested["password"])
	}
	if nested["port"] != 3306 {
		t.Errorf("nested port = %v, want 3306", nested["port"])
	}
}

func TestSnapshotSaveLoad(t *testing.T) {
	dir := t.TempDir()

	snap := Capture("test incident")
	snap.AddError("error", "something failed", "req_1")
	snap.SetHealth("php-fpm", "ok")

	path, err := snap.Save(dir)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.ID != snap.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, snap.ID)
	}
	if loaded.Reason != "test incident" {
		t.Errorf("reason = %q", loaded.Reason)
	}
	if len(loaded.Errors) != 1 {
		t.Errorf("errors = %d, want 1", len(loaded.Errors))
	}
}

func TestListSnapshots(t *testing.T) {
	dir := t.TempDir()

	Capture("incident 1").Save(dir)
	Capture("incident 2").Save(dir)
	os.WriteFile(filepath.Join(dir, "not-a-snapshot.txt"), []byte("ignore"), 0644)

	snaps, err := ListSnapshots(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(snaps) != 2 {
		t.Errorf("snapshots = %d, want 2", len(snaps))
	}
}

func TestLoadSnapshotNonexistent(t *testing.T) {
	_, err := LoadSnapshot("/nonexistent/snapshot.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestSnapshotInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not json"), 0644)

	_, err := LoadSnapshot(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRedactConfigDeepNested(t *testing.T) {
	snap := Capture("test")
	snap.SetConfig(map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"api_key": "abcdef",
				"data":    "safe",
			},
		},
	})

	l1 := snap.Config["level1"].(map[string]interface{})
	l2 := l1["level2"].(map[string]interface{})

	if l2["api_key"] != "***REDACTED***" {
		t.Errorf("deep api_key should be redacted, got %v", l2["api_key"])
	}
	if l2["data"] != "safe" {
		t.Errorf("data should not be redacted, got %v", l2["data"])
	}
}
