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
