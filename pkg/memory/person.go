package memory

import (
	"strings"
	"time"
)

// PersonMemory stores facts about a person.
type PersonMemory struct {
	Name     string    `json:"name"`
	Facts    []string  `json:"facts"`
	LastSeen time.Time `json:"last_seen"`
}

// NewPerson creates a new PersonMemory with the given name.
func NewPerson(name string) *PersonMemory {
	return &PersonMemory{
		Name:     strings.ToLower(strings.TrimSpace(name)),
		Facts:    []string{},
		LastSeen: time.Now(),
	}
}

// AddFact adds a fact to the person's memory.
func (p *PersonMemory) AddFact(fact string) {
	p.Facts = append(p.Facts, fact)
	p.LastSeen = time.Now()
}

// HasFact checks if the person has a specific fact (case-insensitive).
func (p *PersonMemory) HasFact(query string) bool {
	query = strings.ToLower(query)
	for _, fact := range p.Facts {
		if strings.Contains(strings.ToLower(fact), query) {
			return true
		}
	}
	return false
}

// FactCount returns the number of facts stored about this person.
func (p *PersonMemory) FactCount() int {
	return len(p.Facts)
}

// Touch updates the LastSeen timestamp to now.
func (p *PersonMemory) Touch() {
	p.LastSeen = time.Now()
}

// TimeSinceLastSeen returns the duration since the person was last seen.
func (p *PersonMemory) TimeSinceLastSeen() time.Duration {
	return time.Since(p.LastSeen)
}

// --- Memory collection methods for People ---

// RememberPerson stores a fact about a person and auto-saves.
func (m *Memory) RememberPerson(name, fact string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || fact == "" {
		return
	}

	m.mu.Lock()
	if _, ok := m.People[name]; !ok {
		m.People[name] = NewPerson(name)
	}
	m.People[name].AddFact(fact)
	m.mu.Unlock()

	m.Save()
}

// RecallPerson retrieves facts about a person.
func (m *Memory) RecallPerson(name string) []string {
	name = strings.ToLower(strings.TrimSpace(name))

	m.mu.Lock()
	defer m.mu.Unlock()

	if person, ok := m.People[name]; ok {
		person.Touch()
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

	m.mu.RLock()
	defer m.mu.RUnlock()

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

// GetPerson retrieves a person by exact name.
func (m *Memory) GetPerson(name string) *PersonMemory {
	name = strings.ToLower(strings.TrimSpace(name))

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.People[name]
}

// GetAllPeople returns names of all known people.
func (m *Memory) GetAllPeople() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.People))
	for name := range m.People {
		names = append(names, name)
	}
	return names
}

// ForgetPerson removes a person from memory.
func (m *Memory) ForgetPerson(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))

	m.mu.Lock()
	_, exists := m.People[name]
	if exists {
		delete(m.People, name)
	}
	m.mu.Unlock()

	if exists {
		m.Save()
	}
	return exists
}
