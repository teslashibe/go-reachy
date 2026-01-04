package memory

import (
	"strings"
	"time"
)

// Location represents a known place or area in the environment.
type Location struct {
	// Direction relative to current position (left, right, forward, behind)
	Direction string `json:"direction"`

	// Distance estimate (here, nearby, far)
	Distance string `json:"distance,omitempty"`

	// Description of the location
	Description string `json:"description,omitempty"`

	// LastMentioned is when this location was last referenced
	LastMentioned time.Time `json:"last_mentioned,omitempty"`
}

// NewLocation creates a new Location with direction and description.
func NewLocation(direction, description string) *Location {
	return &Location{
		Direction:     direction,
		Description:   description,
		LastMentioned: time.Now(),
	}
}

// Touch updates the LastMentioned timestamp.
func (l *Location) Touch() {
	l.LastMentioned = time.Now()
}

// --- Memory methods for Spatial (location storage) ---

// SetLocation stores a location and auto-saves.
func (m *Memory) SetLocation(name string, loc *Location) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || loc == nil {
		return
	}

	loc.LastMentioned = time.Now()

	m.mu.Lock()
	m.Spatial[name] = loc
	m.mu.Unlock()

	m.Save()
}

// RememberLocation is a convenience method to store a location by its properties.
func (m *Memory) RememberLocation(name, direction, description string) {
	loc := NewLocation(direction, description)
	m.SetLocation(name, loc)
}

// GetLocation retrieves a location by exact name.
func (m *Memory) GetLocation(name string) *Location {
	name = strings.ToLower(strings.TrimSpace(name))

	m.mu.RLock()
	defer m.mu.RUnlock()

	if loc, ok := m.Spatial[name]; ok {
		loc.Touch()
		return loc
	}
	return nil
}

// FindLocation searches for a location by partial name match.
// Returns the name and location if found.
func (m *Memory) FindLocation(query string) (string, *Location) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return "", nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Exact match first
	if loc, ok := m.Spatial[query]; ok {
		return query, loc
	}

	// Partial match
	for name, loc := range m.Spatial {
		if strings.Contains(name, query) {
			return name, loc
		}
	}

	return "", nil
}

// GetAllLocations returns names of all known locations.
func (m *Memory) GetAllLocations() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.Spatial))
	for name := range m.Spatial {
		names = append(names, name)
	}
	return names
}

// ForgetLocation removes a location from memory.
func (m *Memory) ForgetLocation(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))

	m.mu.Lock()
	_, exists := m.Spatial[name]
	if exists {
		delete(m.Spatial, name)
	}
	m.mu.Unlock()

	if exists {
		m.Save()
	}
	return exists
}

// GetLocationsByDirection returns all locations in a given direction.
func (m *Memory) GetLocationsByDirection(direction string) map[string]*Location {
	direction = strings.ToLower(strings.TrimSpace(direction))

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Location)
	for name, loc := range m.Spatial {
		if strings.ToLower(loc.Direction) == direction {
			result[name] = loc
		}
	}
	return result
}


