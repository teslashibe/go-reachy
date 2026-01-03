package memory

import "strings"

// --- Memory methods for Context (situational key-value storage) ---

// SetContext stores a situational fact and auto-saves.
// Examples: SetContext("owner_name", "Brendan"), SetContext("mood", "cheerful")
func (m *Memory) SetContext(key, value string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}

	m.mu.Lock()
	m.Context[key] = value
	m.mu.Unlock()

	m.Save()
}

// GetContext retrieves a situational fact.
// Returns the value and whether it was found.
func (m *Memory) GetContext(key string) (string, bool) {
	key = strings.TrimSpace(key)

	m.mu.RLock()
	defer m.mu.RUnlock()

	value, ok := m.Context[key]
	return value, ok
}

// DeleteContext removes a situational fact and auto-saves.
func (m *Memory) DeleteContext(key string) bool {
	key = strings.TrimSpace(key)

	m.mu.Lock()
	_, exists := m.Context[key]
	if exists {
		delete(m.Context, key)
	}
	m.mu.Unlock()

	if exists {
		m.Save()
	}
	return exists
}

// GetAllContext returns a copy of all context key-value pairs.
func (m *Memory) GetAllContext() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string, len(m.Context))
	for k, v := range m.Context {
		result[k] = v
	}
	return result
}

// GetContextKeys returns all context keys.
func (m *Memory) GetContextKeys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.Context))
	for k := range m.Context {
		keys = append(keys, k)
	}
	return keys
}

// HasContext checks if a context key exists.
func (m *Memory) HasContext(key string) bool {
	key = strings.TrimSpace(key)

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.Context[key]
	return ok
}

// SearchContext finds context keys containing the query (case-insensitive).
func (m *Memory) SearchContext(query string) map[string]string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string)
	for k, v := range m.Context {
		if strings.Contains(strings.ToLower(k), query) ||
			strings.Contains(strings.ToLower(v), query) {
			result[k] = v
		}
	}
	return result
}

