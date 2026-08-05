package state

import (
	"errors"
)

var (
	ErrKeyNotFound = errors.New("state: key not found")
	ErrStoreClosed = errors.New("state: store is closed")
)

// Store defines the state store interface for persisting gateway state.
type Store interface {
	Get(key string) ([]byte, error)
	Set(key string, val []byte) error
	Delete(key string) error
	ListKeys(prefix string) ([]string, error)
	Close() error
}
