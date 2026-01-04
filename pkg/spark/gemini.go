package spark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GeminiClient provides AI-powered title and tag generation for sparks.
type GeminiClient struct {
	apiKey string

	// Rate limiting
	rateMu      sync.Mutex
	lastRequest time.Time
	minInterval time.Duration // Minimum time between requests

	// Simple cache
	cacheMu    sync.RWMutex
	titleCache map[string]string   // content hash -> title
	tagsCache  map[string][]string // content hash -> tags
}

// GeminiConfig configures the Gemini client.
type GeminiConfig struct {
	APIKey         string
	MaxRequestsMin int // Max requests per minute (default: 10)
}

// NewGeminiClient creates a new Gemini client for spark title/tag generation.
func NewGeminiClient(cfg GeminiConfig) *GeminiClient {
	maxReq := cfg.MaxRequestsMin
	if maxReq <= 0 {
		maxReq = 10 // Default: 10 requests per minute
	}

	return &GeminiClient{
		apiKey:      cfg.APIKey,
		minInterval: time.Minute / time.Duration(maxReq),
		titleCache:  make(map[string]string),
		tagsCache:   make(map[string][]string),
	}
}

// GenerateTitle creates a concise, descriptive title for spark content.
// Returns a default title on failure (first 50 chars of content).
func (g *GeminiClient) GenerateTitle(content string) (string, error) {
	if content == "" {
		return "", fmt.Errorf("content is empty")
	}

	// Check cache first
	cacheKey := hashContent(content)
	g.cacheMu.RLock()
	if cached, ok := g.titleCache[cacheKey]; ok {
		g.cacheMu.RUnlock()
		return cached, nil
	}
	g.cacheMu.RUnlock()

	// Rate limit
	if err := g.waitForRateLimit(); err != nil {
		return generateFallbackTitle(content), err
	}

	// If no API key, use fallback
	if g.apiKey == "" {
		return generateFallbackTitle(content), nil
	}

	prompt := fmt.Sprintf(`Generate a concise, descriptive title (3-6 words max) for this idea:

"%s"

Rules:
- Be specific and descriptive
- Capture the essence of the idea
- Use title case
- No quotes or punctuation at the end
- Just output the title, nothing else`, content)

	title, err := g.callGemini(prompt)
	if err != nil {
		return generateFallbackTitle(content), err
	}

	// Clean up the title
	title = cleanTitle(title)

	// Cache the result
	g.cacheMu.Lock()
	g.titleCache[cacheKey] = title
	g.cacheMu.Unlock()

	return title, nil
}

// GenerateTags extracts 3-5 relevant tags for spark content.
// Returns empty slice on failure.
func (g *GeminiClient) GenerateTags(content string) ([]string, error) {
	if content == "" {
		return nil, fmt.Errorf("content is empty")
	}

	// Check cache first
	cacheKey := hashContent(content)
	g.cacheMu.RLock()
	if cached, ok := g.tagsCache[cacheKey]; ok {
		g.cacheMu.RUnlock()
		return cached, nil
	}
	g.cacheMu.RUnlock()

	// Rate limit
	if err := g.waitForRateLimit(); err != nil {
		return nil, err
	}

	// If no API key, return empty
	if g.apiKey == "" {
		return nil, nil
	}

	prompt := fmt.Sprintf(`Extract 3-5 relevant tags for this idea:

"%s"

Rules:
- Tags should be single words or short phrases
- Use lowercase
- Focus on the main topics, technologies, or domains
- Separate tags with commas
- Just output the tags, nothing else

Example output: robotics, gardening, arduino, automation`, content)

	response, err := g.callGemini(prompt)
	if err != nil {
		return nil, err
	}

	// Parse tags from response
	tags := parseTags(response)

	// Cache the result
	g.cacheMu.Lock()
	g.tagsCache[cacheKey] = tags
	g.cacheMu.Unlock()

	return tags, nil
}

// GenerateTitleAndTags generates both title and tags in a single API call.
// More efficient than calling GenerateTitle and GenerateTags separately.
func (g *GeminiClient) GenerateTitleAndTags(content string) (string, []string, error) {
	if content == "" {
		return "", nil, fmt.Errorf("content is empty")
	}

	// Check cache first
	cacheKey := hashContent(content)
	g.cacheMu.RLock()
	cachedTitle, hasTitle := g.titleCache[cacheKey]
	cachedTags, hasTags := g.tagsCache[cacheKey]
	g.cacheMu.RUnlock()

	if hasTitle && hasTags {
		return cachedTitle, cachedTags, nil
	}

	// Rate limit
	if err := g.waitForRateLimit(); err != nil {
		return generateFallbackTitle(content), nil, err
	}

	// If no API key, use fallback
	if g.apiKey == "" {
		return generateFallbackTitle(content), nil, nil
	}

	prompt := fmt.Sprintf(`Analyze this idea and provide a title and tags:

"%s"

Respond in exactly this format:
TITLE: [concise 3-6 word title in Title Case]
TAGS: [3-5 lowercase tags separated by commas]

Example:
TITLE: Smart Plant Watering Robot
TAGS: robotics, gardening, arduino, automation`, content)

	response, err := g.callGemini(prompt)
	if err != nil {
		return generateFallbackTitle(content), nil, err
	}

	// Parse title and tags from response
	title, tags := parseTitleAndTags(response)
	if title == "" {
		title = generateFallbackTitle(content)
	}

	// Cache the results
	g.cacheMu.Lock()
	g.titleCache[cacheKey] = title
	g.tagsCache[cacheKey] = tags
	g.cacheMu.Unlock()

	return title, tags, nil
}

// callGemini makes a request to the Gemini API.
func (g *GeminiClient) callGemini(prompt string) (string, error) {
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.3, // Lower temperature for more consistent outputs
			"maxOutputTokens": 100, // Short responses only
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", g.apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, truncateString(string(bodyBytes), 200))
	}

	var result geminiResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error.Message != "" {
		return "", fmt.Errorf("Gemini error: %s", result.Error.Message)
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text), nil
	}

	return "", fmt.Errorf("no response from Gemini")
}

// waitForRateLimit enforces rate limiting.
func (g *GeminiClient) waitForRateLimit() error {
	g.rateMu.Lock()
	defer g.rateMu.Unlock()

	elapsed := time.Since(g.lastRequest)
	if elapsed < g.minInterval {
		wait := g.minInterval - elapsed
		time.Sleep(wait)
	}

	g.lastRequest = time.Now()
	return nil
}

// geminiResponse is the response structure from Gemini API.
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

// hashContent creates a simple hash key for caching.
func hashContent(content string) string {
	// Simple hash: first 100 chars + length
	if len(content) > 100 {
		return fmt.Sprintf("%s...%d", content[:100], len(content))
	}
	return content
}

// generateFallbackTitle creates a simple title from content.
func generateFallbackTitle(content string) string {
	title := strings.TrimSpace(content)

	// Remove newlines
	title = strings.ReplaceAll(title, "\n", " ")

	// Truncate
	if len(title) > 50 {
		title = title[:47] + "..."
	}

	return title
}

// cleanTitle cleans up a generated title.
func cleanTitle(title string) string {
	// Remove quotes
	title = strings.Trim(title, `"'`)

	// Remove trailing punctuation
	title = strings.TrimRight(title, ".,;:!?")

	// Remove "Title:" prefix if present
	if strings.HasPrefix(strings.ToLower(title), "title:") {
		title = strings.TrimSpace(title[6:])
	}

	// Truncate if too long
	if len(title) > 60 {
		title = title[:57] + "..."
	}

	return strings.TrimSpace(title)
}

// parseTags parses comma-separated tags from a response.
func parseTags(response string) []string {
	// Remove "Tags:" prefix if present
	response = strings.TrimPrefix(strings.ToLower(response), "tags:")
	response = strings.TrimSpace(response)

	// Split by comma
	parts := strings.Split(response, ",")

	var tags []string
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		tag = strings.ToLower(tag)
		tag = strings.Trim(tag, `"'#`)

		if tag != "" && len(tag) < 30 {
			tags = append(tags, tag)
		}
	}

	// Limit to 5 tags
	if len(tags) > 5 {
		tags = tags[:5]
	}

	return tags
}

// parseTitleAndTags parses both title and tags from a combined response.
func parseTitleAndTags(response string) (string, []string) {
	lines := strings.Split(response, "\n")

	var title string
	var tags []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		lineLower := strings.ToLower(line)

		if strings.HasPrefix(lineLower, "title:") {
			title = cleanTitle(strings.TrimSpace(line[6:]))
		} else if strings.HasPrefix(lineLower, "tags:") {
			tags = parseTags(strings.TrimSpace(line[5:]))
		}
	}

	return title, tags
}

// truncateString shortens a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ClearCache clears the title and tag cache.
func (g *GeminiClient) ClearCache() {
	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()
	g.titleCache = make(map[string]string)
	g.tagsCache = make(map[string][]string)
}

// CacheStats returns the number of cached entries.
func (g *GeminiClient) CacheStats() (titles, tags int) {
	g.cacheMu.RLock()
	defer g.cacheMu.RUnlock()
	return len(g.titleCache), len(g.tagsCache)
}

