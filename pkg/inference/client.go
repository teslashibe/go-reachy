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

const providerClient = "client"

// Client is the standard HTTP-based inference provider.
// Works with any OpenAI-compatible API (OpenAI, Ollama, vLLM, Together, Groq, etc.).
type Client struct {
	baseURL string
	apiKey  string
	config  *Config
	http    *http.Client
	logger  *slog.Logger
}

// NewClient creates a new inference client.
func NewClient(opts ...Option) (*Client, error) {
	cfg := DefaultConfig()
	cfg.Apply(opts...)

	baseURL := strings.TrimSuffix(cfg.BaseURL, "/")

	return &Client{
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		config:  cfg,
		http:    &http.Client{Timeout: cfg.Timeout},
		logger:  cfg.Logger.With("component", "inference.client"),
	}, nil
}

// Chat generates a chat completion.
func (c *Client) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	start := time.Now()

	model := req.Model
	if model == "" {
		model = c.config.Model
	}

	payload := c.buildChatPayload(req, model, false)

	resp, err := c.post(ctx, "/chat/completions", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, WrapError(providerClient, fmt.Errorf("decode response: %w", err))
	}

	if len(result.Choices) == 0 {
		return nil, WrapError(providerClient, fmt.Errorf("no choices returned"))
	}

	choice := result.Choices[0]

	return &ChatResponse{
		Message: Message{
			Role:      RoleAssistant,
			Content:   choice.Message.Content,
			ToolCalls: c.parseToolCalls(choice.Message.ToolCalls),
		},
		FinishReason: choice.FinishReason,
		Usage: Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
		Model:     result.Model,
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}

// Vision analyzes an image with a prompt.
func (c *Client) Vision(ctx context.Context, req *VisionRequest) (*VisionResponse, error) {
	start := time.Now()

	model := req.Model
	if model == "" {
		model = c.config.VisionModel
	}

	// Build content array with text and images
	content := []map[string]interface{}{
		{"type": "text", "text": req.Prompt},
	}

	// Add single image
	if req.Image != nil {
		b64, err := EncodeImageBase64(req.Image)
		if err != nil {
			return nil, WrapError(providerClient, fmt.Errorf("encode image: %w", err))
		}
		content = append(content, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]string{
				"url": "data:image/jpeg;base64," + b64,
			},
		})
	}

	// Add multiple images
	for _, img := range req.Images {
		b64, err := EncodeImageBase64(img)
		if err != nil {
			return nil, WrapError(providerClient, fmt.Errorf("encode image: %w", err))
		}
		content = append(content, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]string{
				"url": "data:image/jpeg;base64," + b64,
			},
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 500
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{{
			"role":    "user",
			"content": content,
		}},
		"max_tokens": maxTokens,
	}

	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}

	resp, err := c.post(ctx, "/chat/completions", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, WrapError(providerClient, fmt.Errorf("decode response: %w", err))
	}

	if len(result.Choices) == 0 {
		return nil, WrapError(providerClient, fmt.Errorf("no choices returned"))
	}

	return &VisionResponse{
		Content: result.Choices[0].Message.Content,
		Usage: Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
		Model:     result.Model,
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}

// Embed generates text embeddings.
func (c *Client) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	start := time.Now()

	model := req.Model
	if model == "" {
		model = c.config.EmbedModel
	}

	payload := map[string]interface{}{
		"model": model,
		"input": req.Input,
	}

	resp, err := c.post(ctx, "/embeddings", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, WrapError(providerClient, fmt.Errorf("decode response: %w", err))
	}

	embeddings := make([][]float64, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}

	return &EmbedResponse{
		Embeddings: embeddings,
		Usage: Usage{
			PromptTokens: result.Usage.PromptTokens,
			TotalTokens:  result.Usage.TotalTokens,
		},
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}

// Capabilities returns what this client supports.
func (c *Client) Capabilities() Capabilities {
	return Capabilities{
		Chat:       true,
		Vision:     true,
		Streaming:  true,
		Tools:      true,
		Embeddings: true,
	}
}

// Health checks API connectivity.
func (c *Client) Health(ctx context.Context) error {
	resp, err := c.get(ctx, "/models")
	if err != nil {
		return WrapError(providerClient, fmt.Errorf("health check: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}
	return nil
}

// Close releases resources.
func (c *Client) Close() error {
	c.http.CloseIdleConnections()
	return nil
}

// buildChatPayload constructs the API request payload.
func (c *Client) buildChatPayload(req *ChatRequest, model string, stream bool) map[string]interface{} {
	messages := make([]map[string]interface{}, len(req.Messages))
	for i, msg := range req.Messages {
		m := map[string]interface{}{
			"role":    string(msg.Role),
			"content": msg.Content,
		}
		if msg.Name != "" {
			m["name"] = msg.Name
		}
		if msg.ToolCallID != "" {
			m["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			toolCalls := make([]map[string]interface{}, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				toolCalls[j] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]string{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				}
			}
			m["tool_calls"] = toolCalls
		}
		messages[i] = m
	}

	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}

	if stream {
		payload["stream"] = true
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.config.MaxTokens
	}
	if maxTokens > 0 {
		payload["max_tokens"] = maxTokens
	}

	temp := req.Temperature
	if temp == 0 {
		temp = c.config.Temperature
	}
	if temp > 0 {
		payload["temperature"] = temp
	}

	if req.TopP > 0 {
		payload["top_p"] = req.TopP
	}

	if len(req.Stop) > 0 {
		payload["stop"] = req.Stop
	}

	if len(req.Tools) > 0 {
		tools := make([]map[string]interface{}, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]interface{}{
				"type": t.Type,
				"function": map[string]interface{}{
					"name":        t.Function.Name,
					"description": t.Function.Description,
					"parameters":  t.Function.Parameters,
				},
			}
		}
		payload["tools"] = tools
	}

	if req.ToolChoice != "" {
		payload["tool_choice"] = req.ToolChoice
	}

	return payload
}

// post makes a POST request.
func (c *Client) post(ctx context.Context, path string, payload interface{}) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, WrapError(providerClient, fmt.Errorf("marshal payload: %w", err))
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, WrapError(providerClient, fmt.Errorf("create request: %w", err))
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	return c.doWithRetry(ctx, req, body)
}

// get makes a GET request.
func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, WrapError(providerClient, fmt.Errorf("create request: %w", err))
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	return c.http.Do(req)
}

// doWithRetry performs the request with retry logic.
func (c *Client) doWithRetry(ctx context.Context, req *http.Request, body []byte) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.config.RetryDelay * time.Duration(attempt)):
			}
			// Reset body for retry
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = WrapError(providerClient, err)
			c.logger.Warn("request failed, retrying",
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}

		// Check if retryable
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = c.parseError(resp)
			c.logger.Warn("retrying request",
				"attempt", attempt+1,
				"status", resp.StatusCode,
			)
			continue
		}

		return resp, nil
	}

	return nil, lastErr
}

// parseError reads and parses an error response.
func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse OpenAI-style error
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	message := string(body)
	code := ""
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		message = errResp.Error.Message
		code = errResp.Error.Code
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    message,
		Code:       code,
		Provider:   providerClient,
	}
}

// parseToolCalls converts API tool calls to our format.
func (c *Client) parseToolCalls(calls []apiToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]ToolCall, len(calls))
	for i, call := range calls {
		result[i] = ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		}
	}
	return result
}

// API response types
type chatCompletionResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string        `json:"role"`
			Content   string        `json:"content"`
			ToolCalls []apiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type apiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// Verify Client implements Provider at compile time.
var _ Provider = (*Client)(nil)

