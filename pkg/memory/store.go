package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// Store defines the interface for memory persistence backends.
// Implementations can store to JSON files, SQLite, PostgreSQL, etc.
type Store interface {
	// Save persists the given data.
	Save(data []byte) error

	// Load retrieves the stored data.
	Load() ([]byte, error)

	// Close releases any resources held by the store.
	Close() error
}

// JSONStore implements Store for file-based JSON persistence.
type JSONStore struct {
	FilePath string
}

// NewJSONStore creates a new JSON file store.
func NewJSONStore(path string) *JSONStore {
	return &JSONStore{FilePath: path}
}

// Save writes data to the JSON file.
func (s *JSONStore) Save(data []byte) error {
	if s.FilePath == "" {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(s.FilePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	}

	if err := os.WriteFile(s.FilePath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// Load reads data from the JSON file.
func (s *JSONStore) Load() ([]byte, error) {
	if s.FilePath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(s.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File doesn't exist yet, that's OK
		}
		return nil, fmt.Errorf("read file: %w", err)
	}

	return data, nil
}

// Close is a no-op for JSON files.
func (s *JSONStore) Close() error {
	return nil
}

// Ensure JSONStore implements Store
var _ Store = (*JSONStore)(nil)




