package state

import (
	"sort"
	"strings"
	"sync"
)

// MemoryStore implements Store using in-memory sync.Map.
type MemoryStore struct {
	mu     sync.RWMutex
	data   map[string][]byte
	closed bool
}

// NewMemoryStore creates an in-memory state store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string][]byte),
	}
}

func (m *MemoryStore) Get(key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrStoreClosed
	}

	val, ok := m.data[key]
	if !ok {
		return nil, ErrKeyNotFound
	}
	cp := make([]byte, len(val))
	copy(cp, val)
	return cp, nil
}

func (m *MemoryStore) Set(key string, val []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrStoreClosed
	}

	cp := make([]byte, len(val))
	copy(cp, val)
	m.data[key] = cp
	return nil
}

func (m *MemoryStore) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrStoreClosed
	}

	delete(m.data, key)
	return nil
}

func (m *MemoryStore) ListKeys(prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrStoreClosed
	}

	var keys []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (m *MemoryStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	m.data = nil
	return nil
}
