// Package memory provides persistence for Eva's long-term memory of people and conversations.
package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PersonMemory stores facts about a person.
type PersonMemory struct {
	Name     string    `json:"name"`
	Facts    []string  `json:"facts"`
	LastSeen time.Time `json:"last_seen"`
}

// Memory stores information about people and conversations.
type Memory struct {
	People   map[string]*PersonMemory `json:"people"`
	FilePath string                   `json:"-"`
}

// New creates a new in-memory store (no persistence).
func New() *Memory {
	return &Memory{
		People: make(map[string]*PersonMemory),
	}
}

// NewWithFile creates a memory store that persists to a file.
// Automatically loads existing memory if the file exists.
func NewWithFile(filePath string) *Memory {
	m := &Memory{
		People:   make(map[string]*PersonMemory),
		FilePath: filePath,
	}
	m.Load() // Load existing memory if file exists
	return m
}

// Save persists memory to file.
func (m *Memory) Save() error {
	if m.FilePath == "" {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(m.FilePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.FilePath, data, 0644)
}

// Load reads memory from file.
func (m *Memory) Load() error {
	if m.FilePath == "" {
		return nil
	}

	data, err := os.ReadFile(m.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's OK
		}
		return err
	}

	return json.Unmarshal(data, m)
}

// RememberPerson stores a fact about a person and auto-saves.
func (m *Memory) RememberPerson(name, fact string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return
	}

	if _, ok := m.People[name]; !ok {
		m.People[name] = &PersonMemory{
			Name:  name,
			Facts: []string{},
		}
	}

	m.People[name].Facts = append(m.People[name].Facts, fact)
	m.People[name].LastSeen = time.Now()

	// Auto-save to file
	m.Save()
}

// RecallPerson retrieves facts about a person.
func (m *Memory) RecallPerson(name string) []string {
	name = strings.ToLower(strings.TrimSpace(name))
	if person, ok := m.People[name]; ok {
		person.LastSeen = time.Now()
		return person.Facts
	}
	return nil
}

// FindPerson searches for a person by partial name match.
func (m *Memory) FindPerson(query string) *PersonMemory {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	// Exact match first
	if person, ok := m.People[query]; ok {
		return person
	}

	// Partial match
	for name, person := range m.People {
		if strings.Contains(name, query) {
			return person
		}
	}

	return nil
}

// GetAllPeople returns names of all known people.
func (m *Memory) GetAllPeople() []string {
	names := make([]string, 0, len(m.People))
	for name := range m.People {
		names = append(names, name)
	}
	return names
}

// ToJSON serializes memory to JSON.
func (m *Memory) ToJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// FromJSON deserializes memory from JSON.
func (m *Memory) FromJSON(data []byte) error {
	return json.Unmarshal(data, m)
}


