package diagnostics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Snapshot captures a diagnostic incident bundle.
type Snapshot struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Reason    string                 `json:"reason"`
	Gateway   GatewayInfo            `json:"gateway"`
	Runtime   RuntimeInfo            `json:"runtime"`
	System    SystemInfo             `json:"system"`
	Errors    []ErrorEntry           `json:"errors"`
	Health    map[string]string      `json:"health"`
	Config    map[string]interface{} `json:"config"`
}

// GatewayInfo holds gateway build and version info.
type GatewayInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
}

// RuntimeInfo holds PHP runtime info.
type RuntimeInfo struct {
	Version    string   `json:"version"`
	RuntimeID  string   `json:"runtime_id"`
	Extensions []string `json:"extensions"`
}

// SystemInfo holds system-level info.
type SystemInfo struct {
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	NumCPU       int    `json:"num_cpu"`
	NumGoroutine int    `json:"num_goroutine"`
	MemAlloc     uint64 `json:"mem_alloc_bytes"`
	MemSys       uint64 `json:"mem_sys_bytes"`
	Hostname     string `json:"hostname"`
	PID          int    `json:"pid"`
}

// ErrorEntry is a recorded error.
type ErrorEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	RequestID string    `json:"request_id,omitempty"`
}

// Capture creates a diagnostic snapshot.
func Capture(reason string) *Snapshot {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	hostname, _ := os.Hostname()

	return &Snapshot{
		ID:        fmt.Sprintf("inc_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Reason:    reason,
		Gateway: GatewayInfo{
			GoVersion: runtime.Version(),
		},
		System: SystemInfo{
			OS:           runtime.GOOS,
			Arch:         runtime.GOARCH,
			NumCPU:       runtime.NumCPU(),
			NumGoroutine: runtime.NumGoroutine(),
			MemAlloc:     m.Alloc,
			MemSys:       m.Sys,
			Hostname:     hostname,
			PID:          os.Getpid(),
		},
		Health: make(map[string]string),
	}
}

// AddError records an error in the snapshot.
func (s *Snapshot) AddError(level, message, requestID string) {
	s.Errors = append(s.Errors, ErrorEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		RequestID: requestID,
	})
}

// SetHealth records a health check result.
func (s *Snapshot) SetHealth(component, status string) {
	s.Health[component] = status
}

// SetConfig records config info (redacted).
func (s *Snapshot) SetConfig(cfg map[string]interface{}) {
	s.Config = redactConfig(cfg)
}

// Save writes the snapshot to a file.
func (s *Snapshot) Save(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}

	path := filepath.Join(dir, s.ID+".json")

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}

	return path, nil
}

// LoadSnapshot reads a snapshot from a file.
func LoadSnapshot(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}

	return &snap, nil
}

// ListSnapshots returns all snapshots in a directory.
func ListSnapshots(dir string) ([]*Snapshot, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var snaps []*Snapshot
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		snap, err := LoadSnapshot(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		snaps = append(snaps, snap)
	}

	return snaps, nil
}

// redactConfig removes sensitive values from config.
func redactConfig(cfg map[string]interface{}) map[string]interface{} {
	redacted := make(map[string]interface{})
	sensitive := map[string]bool{
		"token":      true,
		"secret":     true,
		"password":   true,
		"api_key":    true,
		"csrf_secret": true,
	}

	for k, v := range cfg {
		if sensitive[k] {
			redacted[k] = "***REDACTED***"
			continue
		}

		// Recurse into maps.
		if subMap, ok := v.(map[string]interface{}); ok {
			redacted[k] = redactConfig(subMap)
		} else {
			redacted[k] = v
		}
	}

	return redacted
}
