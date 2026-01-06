package spark

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testStore creates a temporary store for testing.
func testStore(t *testing.T) (*JSONStore, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "spark-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	path := filepath.Join(tmpDir, "sparks.json")
	store, err := NewJSONStore(path)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

func TestNewJSONStore(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	if store == nil {
		t.Fatal("expected store to be non-nil")
	}

	if store.Count() != 0 {
		t.Errorf("expected empty store, got %d sparks", store.Count())
	}
}

func TestSave(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	spark := NewSpark("", "Build a robot that waters plants")
	err := store.Save(spark)
	if err != nil {
		t.Fatalf("failed to save spark: %v", err)
	}

	// ID should be generated
	if spark.ID == "" {
		t.Error("expected ID to be generated")
	}

	// Timestamps should be set
	if spark.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if spark.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Should be persisted
	if store.Count() != 1 {
		t.Errorf("expected 1 spark, got %d", store.Count())
	}
}

func TestGet(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	// Save a spark
	original := NewSpark("test-id-123", "Test idea")
	original.Title = "Test Spark"
	store.Save(original)

	// Get by ID
	retrieved, err := store.Get("test-id-123")
	if err != nil {
		t.Fatalf("failed to get spark: %v", err)
	}

	if retrieved.Title != "Test Spark" {
		t.Errorf("expected title 'Test Spark', got '%s'", retrieved.Title)
	}

	// Get non-existent
	_, err = store.Get("non-existent")
	if err == nil {
		t.Error("expected error for non-existent spark")
	}
}

func TestGetByTitle(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	spark := NewSpark("id-1", "Content")
	spark.Title = "Plant Robot"
	store.Save(spark)

	// Exact match (case-insensitive)
	retrieved, err := store.GetByTitle("plant robot")
	if err != nil {
		t.Fatalf("failed to get by title: %v", err)
	}
	if retrieved.ID != "id-1" {
		t.Errorf("expected ID 'id-1', got '%s'", retrieved.ID)
	}

	// Non-existent
	_, err = store.GetByTitle("Non Existent Title")
	if err == nil {
		t.Error("expected error for non-existent title")
	}
}

func TestList(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	// Add sparks with different update times
	spark1 := NewSpark("id-1", "First")
	spark1.Title = "First"
	store.Save(spark1)

	time.Sleep(10 * time.Millisecond)

	spark2 := NewSpark("id-2", "Second")
	spark2.Title = "Second"
	store.Save(spark2)

	time.Sleep(10 * time.Millisecond)

	spark3 := NewSpark("id-3", "Third")
	spark3.Title = "Third"
	store.Save(spark3)

	// List should return newest first
	sparks, err := store.List()
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	if len(sparks) != 3 {
		t.Fatalf("expected 3 sparks, got %d", len(sparks))
	}

	if sparks[0].Title != "Third" {
		t.Errorf("expected first spark to be 'Third', got '%s'", sparks[0].Title)
	}
}

func TestUpdate(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	spark := NewSpark("id-1", "Original content")
	spark.Title = "Original"
	store.Save(spark)

	originalUpdated := spark.UpdatedAt

	time.Sleep(10 * time.Millisecond)

	// Update
	spark.Title = "Updated"
	err := store.Update(spark)
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	// Verify update
	retrieved, _ := store.Get("id-1")
	if retrieved.Title != "Updated" {
		t.Errorf("expected title 'Updated', got '%s'", retrieved.Title)
	}

	if !retrieved.UpdatedAt.After(originalUpdated) {
		t.Error("expected UpdatedAt to be updated")
	}

	// Update non-existent
	nonExistent := NewSpark("non-existent", "Content")
	err = store.Update(nonExistent)
	if err == nil {
		t.Error("expected error for non-existent spark")
	}
}

func TestDelete(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	spark := NewSpark("id-to-delete", "Delete me")
	store.Save(spark)

	if store.Count() != 1 {
		t.Fatalf("expected 1 spark, got %d", store.Count())
	}

	err := store.Delete("id-to-delete")
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	if store.Count() != 0 {
		t.Errorf("expected 0 sparks, got %d", store.Count())
	}

	// Delete non-existent
	err = store.Delete("non-existent")
	if err == nil {
		t.Error("expected error for non-existent spark")
	}
}

func TestSearch(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	spark1 := NewSpark("id-1", "Build a robot that waters plants")
	spark1.Title = "Plant Robot"
	spark1.Tags = []string{"robotics", "gardening"}
	store.Save(spark1)

	spark2 := NewSpark("id-2", "Create an app for tracking habits")
	spark2.Title = "Habit Tracker"
	spark2.Tags = []string{"mobile", "productivity"}
	store.Save(spark2)

	spark3 := NewSpark("id-3", "Design a new gardening tool")
	spark3.Title = "Garden Tool"
	spark3.Tags = []string{"design", "gardening"}
	store.Save(spark3)

	// Search by title
	results, _ := store.Search("robot")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'robot', got %d", len(results))
	}

	// Search by content
	results, _ = store.Search("waters plants")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'waters plants', got %d", len(results))
	}

	// Search by tag
	results, _ = store.Search("gardening")
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'gardening', got %d", len(results))
	}

	// No results
	results, _ = store.Search("blockchain")
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'blockchain', got %d", len(results))
	}
}

func TestFindByKeyword(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	spark1 := NewSpark("id-1", "Content 1")
	spark1.Title = "Plant Watering Robot"
	store.Save(spark1)

	spark2 := NewSpark("id-2", "Content 2")
	spark2.Title = "Chat Robot Assistant"
	store.Save(spark2)

	spark3 := NewSpark("id-3", "Content 3")
	spark3.Title = "Habit Tracker App"
	store.Save(spark3)

	// Find matching keyword
	results, _ := store.FindByKeyword("robot")
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'robot', got %d", len(results))
	}

	// Case insensitive
	results, _ = store.FindByKeyword("ROBOT")
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'ROBOT', got %d", len(results))
	}

	// No match
	results, _ = store.FindByKeyword("blockchain")
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'blockchain', got %d", len(results))
	}
}

func TestCreateSpark(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	spark, err := store.CreateSpark("A new idea for an app")
	if err != nil {
		t.Fatalf("failed to create spark: %v", err)
	}

	if spark.ID == "" {
		t.Error("expected ID to be generated")
	}

	if spark.RawContent != "A new idea for an app" {
		t.Errorf("expected content to match, got '%s'", spark.RawContent)
	}

	if store.Count() != 1 {
		t.Errorf("expected 1 spark, got %d", store.Count())
	}
}

func TestAddContextToSpark(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	spark := NewSpark("id-1", "Original idea")
	spark.Title = "Test Idea"
	store.Save(spark)

	// Add context by ID
	updated, err := store.AddContextToSpark("id-1", "Additional context", SourceVoice)
	if err != nil {
		t.Fatalf("failed to add context: %v", err)
	}

	if len(updated.Context) != 1 {
		t.Errorf("expected 1 context, got %d", len(updated.Context))
	}

	if updated.Context[0].Content != "Additional context" {
		t.Errorf("expected context content to match")
	}

	if updated.Context[0].Source != SourceVoice {
		t.Errorf("expected source to be voice")
	}

	// Add context by keyword
	updated, err = store.AddContextToSpark("Test", "More context", SourceWeb)
	if err != nil {
		t.Fatalf("failed to add context by keyword: %v", err)
	}

	if len(updated.Context) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(updated.Context))
	}
}

func TestUpdateSparkTitle(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	spark := NewSpark("id-1", "Content")
	spark.Title = "Old Title"
	store.Save(spark)

	// Update by ID
	updated, err := store.UpdateSparkTitle("id-1", "New Title")
	if err != nil {
		t.Fatalf("failed to update title: %v", err)
	}

	if updated.Title != "New Title" {
		t.Errorf("expected title 'New Title', got '%s'", updated.Title)
	}

	// Verify persistence
	retrieved, _ := store.Get("id-1")
	if retrieved.Title != "New Title" {
		t.Errorf("expected persisted title 'New Title', got '%s'", retrieved.Title)
	}
}

func TestPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "spark-persist-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "sparks.json")

	// Create and save
	store1, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	spark := NewSpark("persist-id", "Persistent idea")
	spark.Title = "Persistent Spark"
	spark.Tags = []string{"test", "persistence"}
	store1.Save(spark)

	// Load in new store instance
	store2, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("failed to load store: %v", err)
	}

	if store2.Count() != 1 {
		t.Errorf("expected 1 spark after reload, got %d", store2.Count())
	}

	retrieved, err := store2.Get("persist-id")
	if err != nil {
		t.Fatalf("failed to get spark after reload: %v", err)
	}

	if retrieved.Title != "Persistent Spark" {
		t.Errorf("expected title to persist, got '%s'", retrieved.Title)
	}

	if len(retrieved.Tags) != 2 {
		t.Errorf("expected 2 tags to persist, got %d", len(retrieved.Tags))
	}
}

func TestConcurrentAccess(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	// Concurrent saves
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			spark := NewSpark("", "Concurrent idea")
			spark.Title = "Concurrent"
			store.Save(spark)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	if store.Count() != 10 {
		t.Errorf("expected 10 sparks, got %d", store.Count())
	}
}

func TestSparkMethods(t *testing.T) {
	spark := NewSpark("test-id", "Test content")
	spark.Title = "Test"

	// Test AddContext
	spark.AddContext("Context 1", SourceVoice)
	if spark.ContextCount() != 1 {
		t.Errorf("expected 1 context, got %d", spark.ContextCount())
	}

	// Test SetTitle
	spark.SetTitle("New Title")
	if spark.Title != "New Title" {
		t.Errorf("expected title 'New Title', got '%s'", spark.Title)
	}

	// Test SetTags
	spark.SetTags([]string{"tag1", "tag2"})
	if len(spark.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(spark.Tags))
	}

	// Test SetPlan
	plan := &Plan{
		Summary: "Test plan",
		Steps:   []string{"Step 1", "Step 2"},
	}
	spark.SetPlan(plan)
	if !spark.HasPlan() {
		t.Error("expected HasPlan to be true")
	}

	// Test sync status
	if spark.IsSynced() {
		t.Error("expected IsSynced to be false initially")
	}

	spark.MarkSynced("doc-123")
	if !spark.IsSynced() {
		t.Error("expected IsSynced to be true after marking synced")
	}

	spark.AddContext("New context", SourceWeb)
	if !spark.NeedsSync() {
		t.Error("expected NeedsSync to be true after adding context")
	}

	// Test MarkSyncError
	spark.MarkSyncError()
	if spark.SyncStatus != SyncError {
		t.Error("expected SyncStatus to be SyncError")
	}

	// Test Summary
	summary := spark.Summary()
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestPluralize(t *testing.T) {
	// Test singular
	result := pluralize(1, "item", "items")
	if result != "1 item" {
		t.Errorf("expected '1 item', got '%s'", result)
	}

	// Test plural
	result = pluralize(5, "item", "items")
	if result != "5 items" {
		t.Errorf("expected '5 items', got '%s'", result)
	}

	// Test zero
	result = pluralize(0, "item", "items")
	if result != "0 items" {
		t.Errorf("expected '0 items', got '%s'", result)
	}
}

func TestUpdateSparkTitleNotFound(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	_, err := store.UpdateSparkTitle("nonexistent", "New Title")
	if err == nil {
		t.Error("expected error for non-existent spark")
	}
}


