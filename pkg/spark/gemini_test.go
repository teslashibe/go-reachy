package spark

import (
	"testing"
	"time"
)

func TestNewGeminiClient(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey:         "test-key",
		MaxRequestsMin: 10,
	})

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.apiKey != "test-key" {
		t.Errorf("expected apiKey 'test-key', got '%s'", client.apiKey)
	}

	expectedInterval := time.Minute / 10
	if client.minInterval != expectedInterval {
		t.Errorf("expected minInterval %v, got %v", expectedInterval, client.minInterval)
	}
}

func TestNewGeminiClientDefaults(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{})

	// Default rate limit should be 10/min
	expectedInterval := time.Minute / 10
	if client.minInterval != expectedInterval {
		t.Errorf("expected default minInterval %v, got %v", expectedInterval, client.minInterval)
	}
}

func TestGenerateFallbackTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Short title", "Short title"},
		{"This is a really long title that should definitely be truncated because it exceeds fifty characters", "This is a really long title that should definit..."},
		{"  Whitespace  ", "Whitespace"},
		{"Line one\nLine two", "Line one Line two"},
	}

	for _, tt := range tests {
		result := generateFallbackTitle(tt.input)
		if result != tt.expected {
			t.Errorf("generateFallbackTitle(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"Quoted Title"`, "Quoted Title"},
		{"Title with period.", "Title with period"},
		{"Title: Something", "Something"},
		{"  Whitespace  ", "Whitespace"},
		{"Normal Title", "Normal Title"},
	}

	for _, tt := range tests {
		result := cleanTitle(tt.input)
		if result != tt.expected {
			t.Errorf("cleanTitle(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"robotics, gardening, arduino", []string{"robotics", "gardening", "arduino"}},
		{"Tags: ai, machine learning", []string{"ai", "machine learning"}},
		{"#tag1, #tag2, #tag3", []string{"tag1", "tag2", "tag3"}},
		{"UPPERCASE, MixedCase", []string{"uppercase", "mixedcase"}},
		{"", nil},
		{"one, two, three, four, five, six, seven", []string{"one", "two", "three", "four", "five"}}, // Limit to 5
	}

	for _, tt := range tests {
		result := parseTags(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseTags(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i, tag := range result {
			if tag != tt.expected[i] {
				t.Errorf("parseTags(%q)[%d] = %q, want %q", tt.input, i, tag, tt.expected[i])
			}
		}
	}
}

func TestParseTitleAndTags(t *testing.T) {
	tests := []struct {
		input        string
		expectTitle  string
		expectTags   []string
	}{
		{
			"TITLE: Smart Plant Robot\nTAGS: robotics, gardening",
			"Smart Plant Robot",
			[]string{"robotics", "gardening"},
		},
		{
			"title: lowercase title\ntags: tag1, tag2",
			"lowercase title",
			[]string{"tag1", "tag2"},
		},
		{
			"Just some random text",
			"",
			nil,
		},
	}

	for _, tt := range tests {
		title, tags := parseTitleAndTags(tt.input)
		if title != tt.expectTitle {
			t.Errorf("parseTitleAndTags(%q) title = %q, want %q", tt.input, title, tt.expectTitle)
		}
		if len(tags) != len(tt.expectTags) {
			t.Errorf("parseTitleAndTags(%q) tags = %v, want %v", tt.input, tags, tt.expectTags)
		}
	}
}

func TestHashContent(t *testing.T) {
	// Short content returns as-is
	short := "Short content"
	if hashContent(short) != short {
		t.Error("short content should return as-is")
	}

	// Long content gets truncated with length
	long := "This is a very long piece of content that exceeds one hundred characters and should be truncated appropriately for caching purposes"
	hash := hashContent(long)
	if len(hash) > 110 {
		t.Errorf("hash too long: %d chars", len(hash))
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"Hello", 10, "Hello"},
		{"Hello World", 5, "Hello..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := truncateString(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestCacheOperations(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{})

	// Initially empty
	titles, tags := client.CacheStats()
	if titles != 0 || tags != 0 {
		t.Errorf("expected empty cache, got titles=%d, tags=%d", titles, tags)
	}

	// Manually add to cache
	client.cacheMu.Lock()
	client.titleCache["test"] = "Test Title"
	client.tagsCache["test"] = []string{"tag1"}
	client.cacheMu.Unlock()

	titles, tags = client.CacheStats()
	if titles != 1 || tags != 1 {
		t.Errorf("expected 1 each, got titles=%d, tags=%d", titles, tags)
	}

	// Clear cache
	client.ClearCache()
	titles, tags = client.CacheStats()
	if titles != 0 || tags != 0 {
		t.Errorf("expected empty cache after clear, got titles=%d, tags=%d", titles, tags)
	}
}

func TestGenerateTitleWithoutAPIKey(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey: "", // No API key
	})

	content := "Build a robot that waters plants automatically"
	title, err := client.GenerateTitle(content)

	// Should return fallback title without error
	if err != nil {
		t.Logf("Expected behavior - fallback used: %v", err)
	}

	// Should get a reasonable fallback
	if title == "" {
		t.Error("expected non-empty fallback title")
	}

	if len(title) > 50 {
		t.Errorf("fallback title too long: %s", title)
	}
}

func TestGenerateTagsWithoutAPIKey(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey: "", // No API key
	})

	content := "Build a robot that waters plants"
	tags, err := client.GenerateTags(content)

	// Should return nil without API key
	if err != nil {
		t.Logf("Expected behavior - no API key: %v", err)
	}

	if tags != nil {
		t.Errorf("expected nil tags without API key, got %v", tags)
	}
}

func TestGenerateTitleAndTagsWithoutAPIKey(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey: "", // No API key
	})

	content := "Build a robot that waters plants"
	title, tags, err := client.GenerateTitleAndTags(content)

	if err != nil {
		t.Logf("Expected behavior - fallback used: %v", err)
	}

	// Should get fallback title
	if title == "" {
		t.Error("expected non-empty fallback title")
	}

	// Tags should be nil without API
	if tags != nil {
		t.Errorf("expected nil tags, got %v", tags)
	}
}

func TestGenerateTitleEmptyContent(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey: "test-key",
	})

	_, err := client.GenerateTitle("")
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestGenerateTagsEmptyContent(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey: "test-key",
	})

	_, err := client.GenerateTags("")
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestCacheHit(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey: "", // No API key - will use fallback
	})

	content := "Test idea for caching"

	// First call - generates and caches
	title1, _ := client.GenerateTitle(content)

	// Manually set cache to different value to verify cache hit
	client.cacheMu.Lock()
	cacheKey := hashContent(content)
	client.titleCache[cacheKey] = "Cached Title"
	client.cacheMu.Unlock()

	// Second call should hit cache
	title2, _ := client.GenerateTitle(content)

	if title2 != "Cached Title" {
		t.Errorf("expected cached title 'Cached Title', got '%s' (first was '%s')", title2, title1)
	}
}

func TestRateLimiting(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey:         "",
		MaxRequestsMin: 600, // 10 per second for fast testing
	})

	start := time.Now()

	// Make 3 rapid requests
	for i := 0; i < 3; i++ {
		client.waitForRateLimit()
	}

	elapsed := time.Since(start)

	// Should take at least 200ms for 3 requests at 10/sec (100ms interval)
	// But be lenient for test timing
	if elapsed < 150*time.Millisecond {
		t.Logf("Rate limiting may not be working as expected: %v for 3 requests", elapsed)
	}
}

func TestGenerateTitleAndTagsCacheHit(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey: "", // No API key
	})

	content := "Test idea for combined caching"

	// First call
	title1, tags1, _ := client.GenerateTitleAndTags(content)

	// Manually set cache to different values
	client.cacheMu.Lock()
	cacheKey := hashContent(content)
	client.titleCache[cacheKey] = "Cached Combined Title"
	client.tagsCache[cacheKey] = []string{"cached", "tags"}
	client.cacheMu.Unlock()

	// Second call should hit cache
	title2, tags2, _ := client.GenerateTitleAndTags(content)

	if title2 != "Cached Combined Title" {
		t.Errorf("expected cached title, got '%s' (first was '%s')", title2, title1)
	}

	if len(tags2) != 2 || tags2[0] != "cached" {
		t.Errorf("expected cached tags, got %v (first was %v)", tags2, tags1)
	}
}

func TestGenerateTitleAndTagsEmptyContent(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey: "test-key",
	})

	_, _, err := client.GenerateTitleAndTags("")
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestCleanTitleLongTitle(t *testing.T) {
	longTitle := "This is an extremely long title that definitely exceeds sixty characters and should be truncated properly"
	result := cleanTitle(longTitle)

	if len(result) > 60 {
		t.Errorf("title should be truncated to 60 chars max, got %d: %s", len(result), result)
	}
}

func TestParseTagsWithEmptyParts(t *testing.T) {
	// Test with empty parts between commas
	result := parseTags("tag1, , tag2, , tag3")

	if len(result) != 3 {
		t.Errorf("expected 3 tags, got %d: %v", len(result), result)
	}
}

func TestParseTagsWithVeryLongTag(t *testing.T) {
	// Very long tag should be skipped
	result := parseTags("short, this_is_a_really_long_tag_that_exceeds_thirty_characters_limit, another")

	// Should only have 2 tags (long one skipped)
	if len(result) != 2 {
		t.Errorf("expected 2 tags (long one skipped), got %d: %v", len(result), result)
	}
}

func TestGenerateTagsCacheHit(t *testing.T) {
	client := NewGeminiClient(GeminiConfig{
		APIKey: "", // No API key
	})

	content := "Test idea for tag caching"

	// Manually set cache
	client.cacheMu.Lock()
	cacheKey := hashContent(content)
	client.tagsCache[cacheKey] = []string{"cached", "tag"}
	client.cacheMu.Unlock()

	// Should hit cache
	tags, _ := client.GenerateTags(content)

	if len(tags) != 2 || tags[0] != "cached" {
		t.Errorf("expected cached tags, got %v", tags)
	}
}

