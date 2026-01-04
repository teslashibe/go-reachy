package memory

import (
	"fmt"
	"strings"
)

// --- Memory methods for Knowledge (dynamic agent-created collections) ---

// CreateKnowledge creates a new knowledge topic if it doesn't exist.
// Returns an error if the topic already exists.
func (m *Memory) CreateKnowledge(topic string) error {
	topic = strings.ToLower(strings.TrimSpace(topic))
	if topic == "" {
		return fmt.Errorf("topic name cannot be empty")
	}

	m.mu.Lock()
	if _, exists := m.Knowledge[topic]; exists {
		m.mu.Unlock()
		return fmt.Errorf("topic %q already exists", topic)
	}
	m.Knowledge[topic] = make(map[string]any)
	m.mu.Unlock()

	m.Save()
	return nil
}

// EnsureKnowledge creates a knowledge topic if it doesn't exist.
// Unlike CreateKnowledge, this doesn't return an error if it already exists.
func (m *Memory) EnsureKnowledge(topic string) {
	topic = strings.ToLower(strings.TrimSpace(topic))
	if topic == "" {
		return
	}

	m.mu.Lock()
	if _, exists := m.Knowledge[topic]; !exists {
		m.Knowledge[topic] = make(map[string]any)
	}
	m.mu.Unlock()
}

// DeleteKnowledge removes an entire knowledge topic.
func (m *Memory) DeleteKnowledge(topic string) bool {
	topic = strings.ToLower(strings.TrimSpace(topic))

	m.mu.Lock()
	_, exists := m.Knowledge[topic]
	if exists {
		delete(m.Knowledge, topic)
	}
	m.mu.Unlock()

	if exists {
		m.Save()
	}
	return exists
}

// HasKnowledge checks if a knowledge topic exists.
func (m *Memory) HasKnowledge(topic string) bool {
	topic = strings.ToLower(strings.TrimSpace(topic))

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.Knowledge[topic]
	return exists
}

// ListKnowledge returns all knowledge topic names.
func (m *Memory) ListKnowledge() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	topics := make([]string, 0, len(m.Knowledge))
	for topic := range m.Knowledge {
		topics = append(topics, topic)
	}
	return topics
}

// SetKnowledgeItem stores an item in a knowledge topic.
// Creates the topic if it doesn't exist.
func (m *Memory) SetKnowledgeItem(topic, key string, value any) error {
	topic = strings.ToLower(strings.TrimSpace(topic))
	key = strings.TrimSpace(key)

	if topic == "" {
		return fmt.Errorf("topic cannot be empty")
	}
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	m.mu.Lock()
	if _, exists := m.Knowledge[topic]; !exists {
		m.Knowledge[topic] = make(map[string]any)
	}
	m.Knowledge[topic][key] = value
	m.mu.Unlock()

	m.Save()
	return nil
}

// GetKnowledgeItem retrieves an item from a knowledge topic.
func (m *Memory) GetKnowledgeItem(topic, key string) (any, bool) {
	topic = strings.ToLower(strings.TrimSpace(topic))
	key = strings.TrimSpace(key)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if items, exists := m.Knowledge[topic]; exists {
		value, ok := items[key]
		return value, ok
	}
	return nil, false
}

// DeleteKnowledgeItem removes an item from a knowledge topic.
func (m *Memory) DeleteKnowledgeItem(topic, key string) bool {
	topic = strings.ToLower(strings.TrimSpace(topic))
	key = strings.TrimSpace(key)

	m.mu.Lock()
	items, exists := m.Knowledge[topic]
	if !exists {
		m.mu.Unlock()
		return false
	}

	_, itemExists := items[key]
	if itemExists {
		delete(items, key)
	}
	m.mu.Unlock()

	if itemExists {
		m.Save()
	}
	return itemExists
}

// ListKnowledgeItems returns all keys in a knowledge topic.
func (m *Memory) ListKnowledgeItems(topic string) []string {
	topic = strings.ToLower(strings.TrimSpace(topic))

	m.mu.RLock()
	defer m.mu.RUnlock()

	items, exists := m.Knowledge[topic]
	if !exists {
		return nil
	}

	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	return keys
}

// GetKnowledgeTopic returns all items in a knowledge topic.
func (m *Memory) GetKnowledgeTopic(topic string) map[string]any {
	topic = strings.ToLower(strings.TrimSpace(topic))

	m.mu.RLock()
	defer m.mu.RUnlock()

	items, exists := m.Knowledge[topic]
	if !exists {
		return nil
	}

	// Return a copy
	result := make(map[string]any, len(items))
	for k, v := range items {
		result[k] = v
	}
	return result
}

// --- Convenience methods for string values ---

// RememberKnowledge stores a string value in a knowledge topic.
// This is a convenience wrapper around SetKnowledgeItem for simple string storage.
func (m *Memory) RememberKnowledge(topic, key, value string) error {
	return m.SetKnowledgeItem(topic, key, value)
}

// RecallKnowledge retrieves a string value from a knowledge topic.
// Returns empty string and false if not found or not a string.
func (m *Memory) RecallKnowledge(topic, key string) (string, bool) {
	value, ok := m.GetKnowledgeItem(topic, key)
	if !ok {
		return "", false
	}

	// Try to convert to string
	switch v := value.(type) {
	case string:
		return v, true
	case map[string]any:
		// If it's a map, try to get a "value" field
		if val, ok := v["value"].(string); ok {
			return val, true
		}
	}

	return fmt.Sprintf("%v", value), true
}

// SearchKnowledge searches all topics for keys or values containing the query.
func (m *Memory) SearchKnowledge(query string) map[string]map[string]any {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]map[string]any)

	for topic, items := range m.Knowledge {
		for key, value := range items {
			// Check if key matches
			if strings.Contains(strings.ToLower(key), query) {
				if result[topic] == nil {
					result[topic] = make(map[string]any)
				}
				result[topic][key] = value
				continue
			}

			// Check if value matches (for strings)
			if strVal, ok := value.(string); ok {
				if strings.Contains(strings.ToLower(strVal), query) {
					if result[topic] == nil {
						result[topic] = make(map[string]any)
					}
					result[topic][key] = value
				}
			}
		}
	}

	return result
}


