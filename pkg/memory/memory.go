// Package memory provides persistent knowledge storage for Eva.
//
// Memory is organized into four categories:
//   - Context: Situational key-value facts ("owner_name" -> "Brendan")
//   - Spatial: Known locations and places
//   - People: Information about individuals
//   - Knowledge: Dynamic agent-created topic collections
package memory

import (
	"encoding/json"
	"sync"
)

// Memory is the central knowledge store for Eva.
// All data persists to the configured Store backend.
type Memory struct {
	// Context stores situational key-value facts.
	// Examples: "owner_name", "time_of_day", "current_mood"
	Context map[string]string `json:"context"`

	// Spatial stores known locations and places.
	// Examples: "kitchen", "living_room", "front_door"
	Spatial map[string]*Location `json:"spatial"`

	// People stores information about individuals.
	// Keyed by lowercase name.
	People map[string]*PersonMemory `json:"people"`

	// Knowledge stores dynamic agent-created topic collections.
	// Examples: "recipes", "tasks", "preferences"
	Knowledge map[string]map[string]any `json:"knowledge"`

	// store is the persistence backend (not serialized)
	store Store `json:"-"`

	// mu protects concurrent access
	mu sync.RWMutex `json:"-"`
}

// New creates a new in-memory store (no persistence).
func New() *Memory {
	return &Memory{
		Context:   make(map[string]string),
		Spatial:   make(map[string]*Location),
		People:    make(map[string]*PersonMemory),
		Knowledge: make(map[string]map[string]any),
	}
}

// NewWithStore creates a memory with a custom storage backend.
func NewWithStore(store Store) *Memory {
	m := New()
	m.store = store
	m.Load() // Load existing data if available
	return m
}

// NewWithFile creates a memory that persists to a JSON file.
// This is a convenience wrapper around NewWithStore.
func NewWithFile(path string) *Memory {
	return NewWithStore(NewJSONStore(path))
}

// Save persists memory to the configured store.
func (m *Memory) Save() error {
	if m.store == nil {
		return nil
	}

	m.mu.RLock()
	data, err := json.MarshalIndent(m, "", "  ")
	m.mu.RUnlock()

	if err != nil {
		return err
	}

	return m.store.Save(data)
}

// Load reads memory from the configured store.
func (m *Memory) Load() error {
	if m.store == nil {
		return nil
	}

	data, err := m.store.Load()
	if err != nil {
		return err
	}

	if data == nil {
		return nil // No data yet
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Unmarshal into a temporary struct to preserve existing maps
	var loaded Memory
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}

	// Merge loaded data (don't overwrite if nil)
	if loaded.Context != nil {
		m.Context = loaded.Context
	}
	if loaded.Spatial != nil {
		m.Spatial = loaded.Spatial
	}
	if loaded.People != nil {
		m.People = loaded.People
	}
	if loaded.Knowledge != nil {
		m.Knowledge = loaded.Knowledge
	}

	return nil
}

// Close releases resources held by the store.
func (m *Memory) Close() error {
	if m.store == nil {
		return nil
	}
	return m.store.Close()
}

// ToJSON serializes memory to JSON bytes.
func (m *Memory) ToJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.MarshalIndent(m, "", "  ")
}

// FromJSON deserializes memory from JSON bytes.
func (m *Memory) FromJSON(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return json.Unmarshal(data, m)
}

// Clear resets all memory to empty state.
func (m *Memory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Context = make(map[string]string)
	m.Spatial = make(map[string]*Location)
	m.People = make(map[string]*PersonMemory)
	m.Knowledge = make(map[string]map[string]any)
}

// Stats returns counts of items in each category.
func (m *Memory) Stats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	knowledgeItems := 0
	for _, items := range m.Knowledge {
		knowledgeItems += len(items)
	}

	return map[string]int{
		"context":          len(m.Context),
		"spatial":          len(m.Spatial),
		"people":           len(m.People),
		"knowledge_topics": len(m.Knowledge),
		"knowledge_items":  knowledgeItems,
	}
}
