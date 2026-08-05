package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// SQLiteStore provides persistent key-value storage.
// It stores data persistently on disk with atomic JSON-backed writes.
type SQLiteStore struct {
	mu     sync.RWMutex
	path   string
	data   map[string][]byte
	closed bool
}

// NewSQLiteStore creates a persistent state store at path.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("state: dbPath is required")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("state: mkdir %s: %w", filepath.Dir(dbPath), err)
	}

	s := &SQLiteStore{
		path: dbPath,
		data: make(map[string][]byte),
	}

	// Load existing file if present
	if raw, err := os.ReadFile(dbPath); err == nil && len(raw) > 0 {
		var loaded map[string][]byte
		if err := json.Unmarshal(raw, &loaded); err == nil {
			s.data = loaded
		}
	}

	return s, nil
}

func (s *SQLiteStore) Get(key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	val, ok := s.data[key]
	if !ok {
		return nil, ErrKeyNotFound
	}

	cp := make([]byte, len(val))
	copy(cp, val)
	return cp, nil
}

func (s *SQLiteStore) Set(key string, val []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	cp := make([]byte, len(val))
	copy(cp, val)
	s.data[key] = cp

	return s.persistLocked()
}

func (s *SQLiteStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	delete(s.data, key)
	return s.persistLocked()
}

func (s *SQLiteStore) ListKeys(prefix string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	var keys []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	err := s.persistLocked()
	s.closed = true
	s.data = nil
	return err
}

func (s *SQLiteStore) persistLocked() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmpFile := s.path + ".tmp"
	if err := os.WriteFile(tmpFile, raw, 0600); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}

	if err := os.Rename(tmpFile, s.path); err != nil {
		return fmt.Errorf("atomic rename state: %w", err)
	}

	return nil
}
