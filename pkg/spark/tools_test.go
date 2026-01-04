package spark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testToolsStore creates a temporary store for testing tools.
func testToolsStore(t *testing.T) (*JSONStore, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "spark-tools-test-*")
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

func TestSaveSparkTool(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	cfg := ToolsConfig{Store: store}
	tools := Tools(cfg)

	// Find save_spark tool
	var saveTool Tool
	for _, tool := range tools {
		if tool.Name == "save_spark" {
			saveTool = tool
			break
		}
	}

	// Test saving a spark
	result, err := saveTool.Handler(map[string]interface{}{
		"content": "Build a robot that waters plants based on soil moisture",
	})
	if err != nil {
		t.Fatalf("save_spark failed: %v", err)
	}

	if !strings.Contains(result, "captured your spark") {
		t.Errorf("unexpected result: %s", result)
	}

	// Verify it was saved
	if store.Count() != 1 {
		t.Errorf("expected 1 spark, got %d", store.Count())
	}

	// Test empty content
	result, _ = saveTool.Handler(map[string]interface{}{
		"content": "",
	})
	if !strings.Contains(result, "need to know") {
		t.Errorf("expected error for empty content, got: %s", result)
	}
}

func TestSaveSparkWithTitleGenerator(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	cfg := ToolsConfig{
		Store: store,
		TitleGenerator: func(content string) (string, error) {
			return "AI Generated Title", nil
		},
		TagGenerator: func(content string) ([]string, error) {
			return []string{"robotics", "gardening"}, nil
		},
	}
	tools := Tools(cfg)

	var saveTool Tool
	for _, tool := range tools {
		if tool.Name == "save_spark" {
			saveTool = tool
			break
		}
	}

	result, _ := saveTool.Handler(map[string]interface{}{
		"content": "Build a robot",
	})

	if !strings.Contains(result, "AI Generated Title") {
		t.Errorf("expected generated title in result: %s", result)
	}

	// Verify title and tags were set
	sparks, _ := store.List()
	if len(sparks) != 1 {
		t.Fatalf("expected 1 spark")
	}
	if sparks[0].Title != "AI Generated Title" {
		t.Errorf("expected title 'AI Generated Title', got '%s'", sparks[0].Title)
	}
	if len(sparks[0].Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(sparks[0].Tags))
	}
}

func TestAddContextTool(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	// Create a spark first
	spark := NewSpark("test-id", "Original idea")
	spark.Title = "Plant Robot"
	store.Save(spark)

	cfg := ToolsConfig{Store: store}
	tools := Tools(cfg)

	var addContextTool Tool
	for _, tool := range tools {
		if tool.Name == "add_context" {
			addContextTool = tool
			break
		}
	}

	// Add context by title
	result, err := addContextTool.Handler(map[string]interface{}{
		"spark":   "Plant",
		"context": "Could use Arduino with capacitive sensors",
	})
	if err != nil {
		t.Fatalf("add_context failed: %v", err)
	}

	if !strings.Contains(result, "Added to 'Plant Robot'") {
		t.Errorf("unexpected result: %s", result)
	}

	// Verify context was added
	updated, _ := store.Get("test-id")
	if updated.ContextCount() != 1 {
		t.Errorf("expected 1 context, got %d", updated.ContextCount())
	}

	// Test not found
	result, _ = addContextTool.Handler(map[string]interface{}{
		"spark":   "NonExistent",
		"context": "Some context",
	})
	if !strings.Contains(result, "couldn't find") {
		t.Errorf("expected not found message, got: %s", result)
	}
}

func TestUpdateTitleTool(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	spark := NewSpark("test-id", "Content")
	spark.Title = "Old Title"
	store.Save(spark)

	cfg := ToolsConfig{Store: store}
	tools := Tools(cfg)

	var updateTitleTool Tool
	for _, tool := range tools {
		if tool.Name == "update_title" {
			updateTitleTool = tool
			break
		}
	}

	result, err := updateTitleTool.Handler(map[string]interface{}{
		"spark":     "Old",
		"new_title": "New Awesome Title",
	})
	if err != nil {
		t.Fatalf("update_title failed: %v", err)
	}

	if !strings.Contains(result, "Renamed 'Old Title' to 'New Awesome Title'") {
		t.Errorf("unexpected result: %s", result)
	}

	// Verify title was updated
	updated, _ := store.Get("test-id")
	if updated.Title != "New Awesome Title" {
		t.Errorf("expected title 'New Awesome Title', got '%s'", updated.Title)
	}
}

func TestListSparksTool(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	cfg := ToolsConfig{Store: store}
	tools := Tools(cfg)

	var listTool Tool
	for _, tool := range tools {
		if tool.Name == "list_sparks" {
			listTool = tool
			break
		}
	}

	// Empty list
	result, _ := listTool.Handler(map[string]interface{}{})
	if !strings.Contains(result, "don't have any sparks") {
		t.Errorf("expected empty message, got: %s", result)
	}

	// Add some sparks
	spark1 := NewSpark("id-1", "First")
	spark1.Title = "Plant Robot"
	store.Save(spark1)

	spark2 := NewSpark("id-2", "Second")
	spark2.Title = "Chat Assistant"
	spark2.AddContext("Some context", SourceVoice)
	store.Save(spark2)

	result, _ = listTool.Handler(map[string]interface{}{})
	if !strings.Contains(result, "2 sparks") {
		t.Errorf("expected 2 sparks message, got: %s", result)
	}
	if !strings.Contains(result, "Plant Robot") {
		t.Errorf("expected Plant Robot in list, got: %s", result)
	}
	if !strings.Contains(result, "Chat Assistant") {
		t.Errorf("expected Chat Assistant in list, got: %s", result)
	}
}

func TestViewSparkTool(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	spark := NewSpark("test-id", "Build a smart plant watering system")
	spark.Title = "Plant Robot"
	spark.Tags = []string{"robotics", "gardening"}
	spark.AddContext("Use Arduino", SourceVoice)
	spark.AddContext("Add moisture sensors", SourceWeb)
	store.Save(spark)

	cfg := ToolsConfig{Store: store}
	tools := Tools(cfg)

	var viewTool Tool
	for _, tool := range tools {
		if tool.Name == "view_spark" {
			viewTool = tool
			break
		}
	}

	result, err := viewTool.Handler(map[string]interface{}{
		"spark": "Plant",
	})
	if err != nil {
		t.Fatalf("view_spark failed: %v", err)
	}

	if !strings.Contains(result, "Plant Robot") {
		t.Errorf("expected title in result: %s", result)
	}
	if !strings.Contains(result, "Build a smart plant") {
		t.Errorf("expected content in result: %s", result)
	}
	if !strings.Contains(result, "robotics") {
		t.Errorf("expected tags in result: %s", result)
	}
	if !strings.Contains(result, "Use Arduino") {
		t.Errorf("expected context in result: %s", result)
	}
	if !strings.Contains(result, "Want me to start planning") {
		t.Errorf("expected planning suggestion: %s", result)
	}
}

func TestDeleteSparkTool(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	spark := NewSpark("test-id", "Content")
	spark.Title = "Delete Me"
	store.Save(spark)

	cfg := ToolsConfig{Store: store}
	tools := Tools(cfg)

	var deleteTool Tool
	for _, tool := range tools {
		if tool.Name == "delete_spark" {
			deleteTool = tool
			break
		}
	}

	if store.Count() != 1 {
		t.Fatalf("expected 1 spark before delete")
	}

	result, err := deleteTool.Handler(map[string]interface{}{
		"spark": "Delete",
	})
	if err != nil {
		t.Fatalf("delete_spark failed: %v", err)
	}

	if !strings.Contains(result, "Deleted 'Delete Me'") {
		t.Errorf("unexpected result: %s", result)
	}

	if store.Count() != 0 {
		t.Errorf("expected 0 sparks after delete, got %d", store.Count())
	}
}

func TestSearchSparksTool(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	spark1 := NewSpark("id-1", "Build a robot for gardening")
	spark1.Title = "Garden Robot"
	spark1.Tags = []string{"robotics", "gardening"}
	store.Save(spark1)

	spark2 := NewSpark("id-2", "Create a chat assistant")
	spark2.Title = "Chat Bot"
	spark2.Tags = []string{"ai", "chat"}
	store.Save(spark2)

	spark3 := NewSpark("id-3", "Design a robot helper")
	spark3.Title = "Helper Robot"
	spark3.Tags = []string{"robotics", "assistant"}
	store.Save(spark3)

	cfg := ToolsConfig{Store: store}
	tools := Tools(cfg)

	var searchTool Tool
	for _, tool := range tools {
		if tool.Name == "search_sparks" {
			searchTool = tool
			break
		}
	}

	// Search by content
	result, _ := searchTool.Handler(map[string]interface{}{
		"query": "robot",
	})
	if !strings.Contains(result, "2 sparks") {
		t.Errorf("expected 2 results for 'robot', got: %s", result)
	}

	// Search by tag
	result, _ = searchTool.Handler(map[string]interface{}{
		"query": "gardening",
	})
	if !strings.Contains(result, "1 spark") {
		t.Errorf("expected 1 result for 'gardening', got: %s", result)
	}

	// No results
	result, _ = searchTool.Handler(map[string]interface{}{
		"query": "blockchain",
	})
	if !strings.Contains(result, "No sparks found") {
		t.Errorf("expected no results message, got: %s", result)
	}
}

func TestFuzzyMatching(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	spark1 := NewSpark("id-1", "Content 1")
	spark1.Title = "Plant Watering Robot"
	store.Save(spark1)

	spark2 := NewSpark("id-2", "Content 2")
	spark2.Title = "Chat Robot Assistant"
	store.Save(spark2)

	// Test finding by partial match
	spark, ambiguous, err := findSparkFuzzy(store, "Plant")
	if err != nil {
		t.Fatalf("findSparkFuzzy failed: %v", err)
	}
	if ambiguous != "" {
		t.Errorf("expected no ambiguity, got: %s", ambiguous)
	}
	if spark.Title != "Plant Watering Robot" {
		t.Errorf("expected 'Plant Watering Robot', got '%s'", spark.Title)
	}

	// Test ambiguous match
	_, ambiguous, err = findSparkFuzzy(store, "Robot")
	if err != nil {
		t.Fatalf("findSparkFuzzy failed: %v", err)
	}
	if ambiguous == "" {
		t.Error("expected ambiguous message for 'Robot'")
	}
	if !strings.Contains(ambiguous, "2 sparks") {
		t.Errorf("expected ambiguity count, got: %s", ambiguous)
	}

	// Test not found
	_, _, err = findSparkFuzzy(store, "NonExistent")
	if err == nil {
		t.Error("expected error for non-existent spark")
	}
}

func TestGenerateDefaultTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Short title", "Short title"},
		{"This is a really long title that should be truncated to fifty chars", "This is a really long title that should be trun..."},
		{"  Trim whitespace  ", "Trim whitespace"},
	}

	for _, tt := range tests {
		result := generateDefaultTitle(tt.input)
		if result != tt.expected {
			t.Errorf("generateDefaultTitle(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestToolDescriptions(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	cfg := ToolsConfig{Store: store}
	tools := Tools(cfg)

	expectedTools := []string{
		"save_spark",
		"add_context",
		"update_title",
		"list_sparks",
		"view_spark",
		"delete_spark",
		"search_sparks",
	}

	if len(tools) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d", len(expectedTools), len(tools))
	}

	for _, expected := range expectedTools {
		found := false
		for _, tool := range tools {
			if tool.Name == expected {
				found = true
				if tool.Description == "" {
					t.Errorf("tool %s has empty description", expected)
				}
				break
			}
		}
		if !found {
			t.Errorf("missing expected tool: %s", expected)
		}
	}
}

func TestToolsWithNilStore(t *testing.T) {
	cfg := ToolsConfig{Store: nil}
	tools := Tools(cfg)

	// All tools should handle nil store gracefully
	for _, tool := range tools {
		result, _ := tool.Handler(map[string]interface{}{
			"content":   "test",
			"spark":     "test",
			"context":   "test",
			"new_title": "test",
			"query":     "test",
		})
		if !strings.Contains(result, "not available") {
			// Some tools might ask for which spark, etc
			if !strings.Contains(result, "Which spark") && !strings.Contains(result, "What") {
				t.Logf("Tool %s returned: %s", tool.Name, result)
			}
		}
	}
}

func TestViewSparkWithPlan(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	spark := NewSpark("test-id", "Build something cool")
	spark.Title = "Cool Project"
	spark.SetPlan(&Plan{
		Summary: "A plan to build something cool",
		Steps:   []string{"Step 1: Design", "Step 2: Build", "Step 3: Test"},
	})
	store.Save(spark)

	cfg := ToolsConfig{Store: store}
	tools := Tools(cfg)

	var viewTool Tool
	for _, tool := range tools {
		if tool.Name == "view_spark" {
			viewTool = tool
			break
		}
	}

	result, _ := viewTool.Handler(map[string]interface{}{
		"spark": "Cool",
	})

	if !strings.Contains(result, "Plan:") {
		t.Errorf("expected plan in result: %s", result)
	}
	if !strings.Contains(result, "Step 1: Design") {
		t.Errorf("expected steps in result: %s", result)
	}
}

func TestEmptyParameterHandling(t *testing.T) {
	store, cleanup := testToolsStore(t)
	defer cleanup()

	cfg := ToolsConfig{Store: store}
	tools := Tools(cfg)

	// Test add_context with empty spark
	var addContextTool Tool
	for _, tool := range tools {
		if tool.Name == "add_context" {
			addContextTool = tool
			break
		}
	}

	result, _ := addContextTool.Handler(map[string]interface{}{
		"spark":   "",
		"context": "test",
	})
	if !strings.Contains(result, "Which spark") {
		t.Errorf("expected prompt for spark, got: %s", result)
	}

	result, _ = addContextTool.Handler(map[string]interface{}{
		"spark":   "test",
		"context": "",
	})
	if !strings.Contains(result, "What context") {
		t.Errorf("expected prompt for context, got: %s", result)
	}

	// Test update_title with empty params
	var updateTitleTool Tool
	for _, tool := range tools {
		if tool.Name == "update_title" {
			updateTitleTool = tool
			break
		}
	}

	result, _ = updateTitleTool.Handler(map[string]interface{}{
		"spark":     "",
		"new_title": "test",
	})
	if !strings.Contains(result, "Which spark") {
		t.Errorf("expected prompt for spark, got: %s", result)
	}

	result, _ = updateTitleTool.Handler(map[string]interface{}{
		"spark":     "test",
		"new_title": "",
	})
	if !strings.Contains(result, "What should I rename") {
		t.Errorf("expected prompt for new title, got: %s", result)
	}

	// Test view_spark with empty spark
	var viewTool Tool
	for _, tool := range tools {
		if tool.Name == "view_spark" {
			viewTool = tool
			break
		}
	}

	result, _ = viewTool.Handler(map[string]interface{}{
		"spark": "",
	})
	if !strings.Contains(result, "Which spark") {
		t.Errorf("expected prompt for spark, got: %s", result)
	}

	// Test delete_spark with empty spark
	var deleteTool Tool
	for _, tool := range tools {
		if tool.Name == "delete_spark" {
			deleteTool = tool
			break
		}
	}

	result, _ = deleteTool.Handler(map[string]interface{}{
		"spark": "",
	})
	if !strings.Contains(result, "Which spark") {
		t.Errorf("expected prompt for spark, got: %s", result)
	}

	// Test search_sparks with empty query
	var searchTool Tool
	for _, tool := range tools {
		if tool.Name == "search_sparks" {
			searchTool = tool
			break
		}
	}

	result, _ = searchTool.Handler(map[string]interface{}{
		"query": "",
	})
	if !strings.Contains(result, "What do you want to search") {
		t.Errorf("expected prompt for query, got: %s", result)
	}
}

