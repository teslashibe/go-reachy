package spark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store defines the interface for spark storage operations.
type Store interface {
	// Save creates or updates a spark
	Save(spark *Spark) error

	// Get retrieves a spark by ID
	Get(id string) (*Spark, error)

	// GetByTitle retrieves a spark by exact title match
	GetByTitle(title string) (*Spark, error)

	// List returns all sparks, sorted by updated time (newest first)
	List() ([]*Spark, error)

	// Update updates an existing spark
	Update(spark *Spark) error

	// Delete removes a spark by ID
	Delete(id string) error

	// Search finds sparks matching a query (searches title, content, tags)
	Search(query string) ([]*Spark, error)

	// FindByKeyword finds sparks where title contains the keyword (fuzzy match)
	FindByKeyword(keyword string) ([]*Spark, error)

	// Count returns the total number of sparks
	Count() int
}

// JSONStore implements Store using a JSON file for persistence.
type JSONStore struct {
	path   string
	sparks map[string]*Spark
	mu     sync.RWMutex
}

// storeData is the JSON structure for the store file.
type storeData struct {
	Version   int      `json:"version"`
	UpdatedAt string   `json:"updated_at"`
	Sparks    []*Spark `json:"sparks"`
}

const currentVersion = 1

// NewJSONStore creates a new JSON-based store at the given path.
// If the file doesn't exist, it will be created on first save.
func NewJSONStore(path string) (*JSONStore, error) {
	store := &JSONStore{
		path:   path,
		sparks: make(map[string]*Spark),
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Load existing data if file exists
	if _, err := os.Stat(path); err == nil {
		if err := store.load(); err != nil {
			return nil, fmt.Errorf("failed to load store: %w", err)
		}
	}

	return store, nil
}

// NewDefaultStore creates a store at the default location (~/.eva/sparks.json).
func NewDefaultStore() (*JSONStore, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	path := filepath.Join(homeDir, ".eva", "sparks.json")
	return NewJSONStore(path)
}

// load reads the store from disk.
func (s *JSONStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var stored storeData
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Convert slice to map for fast lookup
	s.sparks = make(map[string]*Spark)
	for _, spark := range stored.Sparks {
		s.sparks[spark.ID] = spark
	}

	return nil
}

// save writes the store to disk.
func (s *JSONStore) save() error {
	// Convert map to slice for storage
	sparks := make([]*Spark, 0, len(s.sparks))
	for _, spark := range s.sparks {
		sparks = append(sparks, spark)
	}

	stored := storeData{
		Version:   currentVersion,
		UpdatedAt: time.Now().Format(time.RFC3339),
		Sparks:    sparks,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to temp file first, then rename (atomic write)
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Save creates or updates a spark.
func (s *JSONStore) Save(spark *Spark) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate ID if not set
	if spark.ID == "" {
		spark.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	if spark.CreatedAt.IsZero() {
		spark.CreatedAt = now
	}
	spark.UpdatedAt = now

	s.sparks[spark.ID] = spark
	return s.save()
}

// Get retrieves a spark by ID.
func (s *JSONStore) Get(id string) (*Spark, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	spark, ok := s.sparks[id]
	if !ok {
		return nil, fmt.Errorf("spark not found: %s", id)
	}
	return spark, nil
}

// GetByTitle retrieves a spark by exact title match (case-insensitive).
func (s *JSONStore) GetByTitle(title string) (*Spark, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	titleLower := strings.ToLower(title)
	for _, spark := range s.sparks {
		if strings.ToLower(spark.Title) == titleLower {
			return spark, nil
		}
	}
	return nil, fmt.Errorf("spark not found with title: %s", title)
}

// List returns all sparks, sorted by updated time (newest first).
func (s *JSONStore) List() ([]*Spark, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sparks := make([]*Spark, 0, len(s.sparks))
	for _, spark := range s.sparks {
		sparks = append(sparks, spark)
	}

	// Sort by UpdatedAt descending
	for i := 0; i < len(sparks)-1; i++ {
		for j := i + 1; j < len(sparks); j++ {
			if sparks[j].UpdatedAt.After(sparks[i].UpdatedAt) {
				sparks[i], sparks[j] = sparks[j], sparks[i]
			}
		}
	}

	return sparks, nil
}

// Update updates an existing spark.
func (s *JSONStore) Update(spark *Spark) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sparks[spark.ID]; !ok {
		return fmt.Errorf("spark not found: %s", spark.ID)
	}

	spark.UpdatedAt = time.Now()
	s.sparks[spark.ID] = spark
	return s.save()
}

// Delete removes a spark by ID.
func (s *JSONStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sparks[id]; !ok {
		return fmt.Errorf("spark not found: %s", id)
	}

	delete(s.sparks, id)
	return s.save()
}

// Search finds sparks matching a query (searches title, content, tags).
func (s *JSONStore) Search(query string) ([]*Spark, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	queryLower := strings.ToLower(query)
	var results []*Spark

	for _, spark := range s.sparks {
		// Search in title
		if strings.Contains(strings.ToLower(spark.Title), queryLower) {
			results = append(results, spark)
			continue
		}

		// Search in raw content
		if strings.Contains(strings.ToLower(spark.RawContent), queryLower) {
			results = append(results, spark)
			continue
		}

		// Search in tags
		for _, tag := range spark.Tags {
			if strings.Contains(strings.ToLower(tag), queryLower) {
				results = append(results, spark)
				break
			}
		}
	}

	return results, nil
}

// FindByKeyword finds sparks where title contains the keyword (fuzzy match).
func (s *JSONStore) FindByKeyword(keyword string) ([]*Spark, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keywordLower := strings.ToLower(keyword)
	var results []*Spark

	for _, spark := range s.sparks {
		if strings.Contains(strings.ToLower(spark.Title), keywordLower) {
			results = append(results, spark)
		}
	}

	return results, nil
}

// Count returns the total number of sparks.
func (s *JSONStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sparks)
}

// Path returns the file path of the store.
func (s *JSONStore) Path() string {
	return s.path
}

// CreateSpark is a convenience method that creates a new spark with the given content.
// It generates a UUID, sets timestamps, and saves to disk.
func (s *JSONStore) CreateSpark(rawContent string) (*Spark, error) {
	spark := NewSpark(uuid.New().String(), rawContent)
	if err := s.Save(spark); err != nil {
		return nil, err
	}
	return spark, nil
}

// AddContextToSpark adds context to a spark identified by ID or title keyword.
func (s *JSONStore) AddContextToSpark(identifier, content string, source ContextSource) (*Spark, error) {
	// Try to find by ID first
	spark, err := s.Get(identifier)
	if err != nil {
		// Try fuzzy match by title
		matches, err := s.FindByKeyword(identifier)
		if err != nil || len(matches) == 0 {
			return nil, fmt.Errorf("spark not found: %s", identifier)
		}
		if len(matches) > 1 {
			// Return first match but caller should handle ambiguity
			spark = matches[0]
		} else {
			spark = matches[0]
		}
	}

	spark.AddContext(content, source)
	if err := s.Update(spark); err != nil {
		return nil, err
	}
	return spark, nil
}

// UpdateSparkTitle updates the title of a spark identified by ID or title keyword.
func (s *JSONStore) UpdateSparkTitle(identifier, newTitle string) (*Spark, error) {
	// Try to find by ID first
	spark, err := s.Get(identifier)
	if err != nil {
		// Try fuzzy match by title
		matches, err := s.FindByKeyword(identifier)
		if err != nil || len(matches) == 0 {
			return nil, fmt.Errorf("spark not found: %s", identifier)
		}
		if len(matches) > 1 {
			spark = matches[0]
		} else {
			spark = matches[0]
		}
	}

	spark.SetTitle(newTitle)
	if err := s.Update(spark); err != nil {
		return nil, err
	}
	return spark, nil
}

