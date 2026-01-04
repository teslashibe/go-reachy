// Package vision provides computer vision capabilities including Gemini-based image analysis.
package vision

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GeminiVision calls Gemini Flash to describe an image.
func GeminiVision(apiKey string, imageData []byte, prompt string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("GOOGLE_API_KEY not set")
	}

	// Encode image as base64
	b64Image := base64.StdEncoding.EncodeToString(imageData)

	// Build Gemini API request
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
					{"inline_data": map[string]string{"mime_type": "image/jpeg", "data": b64Image}},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 1000,
		},
	}

	jsonData, _ := json.Marshal(payload)

	// Using Gemini 2.0 Flash - stable model
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result geminiResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w (body: %s)", err, truncate(string(bodyBytes), 200))
	}

	if result.Error.Message != "" {
		return "", fmt.Errorf("Gemini error: %s", result.Error.Message)
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("no response from Gemini (raw: %s)", truncate(string(bodyBytes), 300))
}

// WebSearch uses Gemini with Google Search grounding to search the web.
func WebSearch(apiKey string, query string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("GOOGLE_API_KEY not set")
	}

	// Build request with search grounding enabled
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": query},
				},
			},
		},
		"tools": []map[string]interface{}{
			{
				"google_search": map[string]interface{}{},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.2,
			"maxOutputTokens": 300,
		},
		"systemInstruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": "You are a helpful assistant that searches the web for real-time information. Always use Google Search to find current, accurate information. Provide specific details like prices, times, dates, and links when available. Be concise but informative."},
			},
		},
	}

	jsonData, _ := json.Marshal(payload)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(bodyBytes, &errResp)
		if errResp.Error.Message != "" {
			return "", fmt.Errorf("Gemini error: %s", errResp.Error.Message)
		}
		return "", fmt.Errorf("Gemini API error (status %d)", resp.StatusCode)
	}

	var result webSearchResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		response := strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)

		// Add source links if available
		metadata := result.Candidates[0].GroundingMetadata
		if len(metadata.GroundingChunks) > 0 {
			response += "\n\nSources: "
			for i, chunk := range metadata.GroundingChunks {
				if i > 2 {
					break
				}
				if chunk.Web.Title != "" {
					response += fmt.Sprintf("%s (%s), ", chunk.Web.Title, chunk.Web.URI)
				}
			}
		}

		return response, nil
	}

	return "", fmt.Errorf("no search results")
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

// webSearchResponse includes grounding metadata.
type webSearchResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		GroundingMetadata struct {
			WebSearchQueries []string `json:"webSearchQueries"`
			SearchEntryPoint struct {
				RenderedContent string `json:"renderedContent"`
			} `json:"searchEntryPoint"`
			GroundingChunks []struct {
				Web struct {
					URI   string `json:"uri"`
					Title string `json:"title"`
				} `json:"web"`
			} `json:"groundingChunks"`
		} `json:"groundingMetadata"`
	} `json:"candidates"`
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}



