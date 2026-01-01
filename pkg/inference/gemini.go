package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const providerGemini = "gemini"

// Gemini implements the Provider interface for Google's Gemini API.
// Note: Gemini uses a different API format than OpenAI, so we implement it directly.
type Gemini struct {
	apiKey string
	config *Config
	http   *http.Client
	logger *slog.Logger
}

// NewGemini creates a Gemini provider.
func NewGemini(opts ...Option) (*Gemini, error) {
	cfg := DefaultConfig()
	cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	cfg.Model = "gemini-2.0-flash"
	cfg.VisionModel = "gemini-2.0-flash"
	cfg.Apply(opts...)

	if cfg.APIKey == "" {
		return nil, WrapError(providerGemini, ErrNoAPIKey)
	}

	return &Gemini{
		apiKey: cfg.APIKey,
		config: cfg,
		http:   &http.Client{Timeout: cfg.Timeout},
		logger: cfg.Logger.With("component", "inference.gemini"),
	}, nil
}

// Chat generates a chat completion using Gemini.
func (g *Gemini) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	start := time.Now()

	model := req.Model
	if model == "" {
		model = g.config.Model
	}

	// Convert messages to Gemini format
	contents := g.convertMessages(req.Messages)

	payload := map[string]interface{}{
		"contents": contents,
		"generationConfig": map[string]interface{}{
			"temperature":     g.config.Temperature,
			"maxOutputTokens": req.MaxTokens,
		},
	}

	if req.MaxTokens == 0 {
		payload["generationConfig"].(map[string]interface{})["maxOutputTokens"] = g.config.MaxTokens
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, WrapError(providerGemini, err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", g.config.BaseURL, model, g.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, WrapError(providerGemini, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(httpReq)
	if err != nil {
		return nil, WrapError(providerGemini, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, g.parseError(resp)
	}

	var result geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, WrapError(providerGemini, fmt.Errorf("decode response: %w", err))
	}

	if result.Error.Message != "" {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    result.Error.Message,
			Provider:   providerGemini,
		}
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return nil, WrapError(providerGemini, fmt.Errorf("no response content"))
	}

	return &ChatResponse{
		Message: Message{
			Role:    RoleAssistant,
			Content: result.Candidates[0].Content.Parts[0].Text,
		},
		FinishReason: result.Candidates[0].FinishReason,
		Model:        model,
		LatencyMs:    time.Since(start).Milliseconds(),
	}, nil
}

// Stream is not yet implemented for Gemini.
func (g *Gemini) Stream(ctx context.Context, req *ChatRequest) (Stream, error) {
	return nil, WrapError(providerGemini, fmt.Errorf("streaming not yet implemented"))
}

// Vision analyzes an image using Gemini.
func (g *Gemini) Vision(ctx context.Context, req *VisionRequest) (*VisionResponse, error) {
	start := time.Now()

	model := req.Model
	if model == "" {
		model = g.config.VisionModel
	}

	// Build content parts
	parts := []map[string]interface{}{
		{"text": req.Prompt},
	}

	// Add single image
	if req.Image != nil {
		b64, err := EncodeImageBase64(req.Image)
		if err != nil {
			return nil, WrapError(providerGemini, fmt.Errorf("encode image: %w", err))
		}
		parts = append(parts, map[string]interface{}{
			"inline_data": map[string]string{
				"mime_type": "image/jpeg",
				"data":      b64,
			},
		})
	}

	// Add multiple images
	for _, img := range req.Images {
		b64, err := EncodeImageBase64(img)
		if err != nil {
			return nil, WrapError(providerGemini, fmt.Errorf("encode image: %w", err))
		}
		parts = append(parts, map[string]interface{}{
			"inline_data": map[string]string{
				"mime_type": "image/jpeg",
				"data":      b64,
			},
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 1000
	}

	temp := req.Temperature
	if temp == 0 {
		temp = 0.7
	}

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": parts},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     temp,
			"maxOutputTokens": maxTokens,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, WrapError(providerGemini, err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", g.config.BaseURL, model, g.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, WrapError(providerGemini, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(httpReq)
	if err != nil {
		return nil, WrapError(providerGemini, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, g.parseError(resp)
	}

	var result geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, WrapError(providerGemini, fmt.Errorf("decode response: %w", err))
	}

	if result.Error.Message != "" {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    result.Error.Message,
			Provider:   providerGemini,
		}
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return nil, WrapError(providerGemini, fmt.Errorf("no response content"))
	}

	return &VisionResponse{
		Content:   result.Candidates[0].Content.Parts[0].Text,
		Model:     model,
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}

// Embed is not supported by this implementation.
func (g *Gemini) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	return nil, WrapError(providerGemini, ErrEmbeddingsNotSupported)
}

// Capabilities returns Gemini's capabilities.
func (g *Gemini) Capabilities() Capabilities {
	return Capabilities{
		Chat:       true,
		Vision:     true,
		Streaming:  false, // Not yet implemented
		Tools:      false, // Gemini has tools but different format
		Embeddings: false,
	}
}

// Health checks API connectivity.
func (g *Gemini) Health(ctx context.Context) error {
	// Simple test call
	_, err := g.Chat(ctx, &ChatRequest{
		Messages:  []Message{NewUserMessage("test")},
		MaxTokens: 1,
	})
	return err
}

// Close releases resources.
func (g *Gemini) Close() error {
	g.http.CloseIdleConnections()
	return nil
}

// convertMessages converts our Message format to Gemini's format.
func (g *Gemini) convertMessages(msgs []Message) []map[string]interface{} {
	var contents []map[string]interface{}

	for _, msg := range msgs {
		role := "user"
		if msg.Role == RoleAssistant {
			role = "model"
		}

		parts := []map[string]interface{}{
			{"text": msg.Content},
		}

		contents = append(contents, map[string]interface{}{
			"role":  role,
			"parts": parts,
		})
	}

	return contents
}

// parseError reads and parses an error response.
func (g *Gemini) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}

	message := string(body)
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		message = errResp.Error.Message
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    message,
		Provider:   providerGemini,
	}
}

// geminiResponse is the Gemini API response format.
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

// GeminiSearch uses Gemini with Google Search grounding for web searches.
func GeminiSearch(ctx context.Context, apiKey, query string) (string, error) {
	if apiKey == "" {
		return "", WrapError(providerGemini, ErrNoAPIKey)
	}

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

	body, err := json.Marshal(payload)
	if err != nil {
		return "", WrapError(providerGemini, err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", WrapError(providerGemini, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", WrapError(providerGemini, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return "", &APIError{StatusCode: resp.StatusCode, Message: errResp.Error.Message, Provider: providerGemini}
		}
		return "", &APIError{StatusCode: resp.StatusCode, Message: string(respBody), Provider: providerGemini}
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			GroundingMetadata struct {
				GroundingChunks []struct {
					Web struct {
						URI   string `json:"uri"`
						Title string `json:"title"`
					} `json:"web"`
				} `json:"groundingChunks"`
			} `json:"groundingMetadata"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", WrapError(providerGemini, err)
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		response := strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)

		// Add source links
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

	return "", WrapError(providerGemini, fmt.Errorf("no search results"))
}

// Verify Gemini implements Provider at compile time.
var _ Provider = (*Gemini)(nil)


