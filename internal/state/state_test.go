package state

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestMemoryStore(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	testStoreContract(t, store)
}

func TestSQLiteStore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	testStoreContract(t, store)

	// Verify persistent re-opening
	_ = store.Set("persistent_key", []byte("persistent_val"))

	store2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("re-opening SQLiteStore failed: %v", err)
	}
	defer store2.Close()

	val, err := store2.Get("persistent_key")
	if err != nil {
		t.Fatalf("failed to get persistent key: %v", err)
	}
	if !bytes.Equal(val, []byte("persistent_val")) {
		t.Errorf("got %q, want persistent_val", string(val))
	}
}

func testStoreContract(t *testing.T, store Store) {
	// Key not found
	_, err := store.Get("nonexistent")
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}

	// Set & Get
	if err := store.Set("app:1:name", []byte("wordpress")); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := store.Set("app:2:name", []byte("laravel")); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := store.Get("app:1:name")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(val) != "wordpress" {
		t.Errorf("Get = %q, want wordpress", string(val))
	}

	// ListKeys
	keys, err := store.ListKeys("app:")
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}
	if len(keys) != 2 || keys[0] != "app:1:name" || keys[1] != "app:2:name" {
		t.Errorf("ListKeys = %v, want [app:1:name, app:2:name]", keys)
	}

	// Delete
	if err := store.Delete("app:1:name"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err = store.Get("app:1:name")
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound after Delete, got %v", err)
	}
}
